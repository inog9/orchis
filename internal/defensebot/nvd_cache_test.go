package defensebot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func nvdWith(t *testing.T, dir string, now func() time.Time, status int) (*NVD, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"totalResults":0,"vulnerabilities":[]}`)) // NVD: 200 dengan 0 hasil = memang tidak ada CVE
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return &NVD{Base: srv.URL, Sleep: func(time.Duration) {}, CacheDir: dir, Now: now}, &hits
}

// Hasil sukses (termasuk "tidak ada CVE") disimpan di disk dan dipakai lagi oleh instans baru (restart worker).
func TestNVDCacheSurvivesRestartAndSkipsTheNetwork(t *testing.T) {
	dir := t.TempDir()
	a, hitsA := nvdWith(t, dir, nil, http.StatusOK)
	if _, err := a.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(hitsA) != 1 {
		t.Fatalf("first lookup must hit NVD once, got %d", *hitsA)
	}
	b, hitsB := nvdWith(t, dir, nil, http.StatusServiceUnavailable) // NVD "mati": tidak boleh dipanggil
	if _, err := b.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatalf("cached lookup must not fail while NVD is down: %v", err)
	}
	if atomic.LoadInt32(hitsB) != 0 {
		t.Fatalf("a cached lookup must not call NVD, got %d calls", *hitsB)
	}
}

// Entri kedaluwarsa tidak dipakai, dan kegagalan tidak pernah disimpan sebagai hasil.
func TestNVDCacheExpiresAndNeverStoresFailures(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	a, _ := nvdWith(t, dir, func() time.Time { return t0 }, http.StatusOK)
	if _, err := a.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	later := t0.Add(DefaultNVDCacheTTL + time.Hour)
	b, hits := nvdWith(t, dir, func() time.Time { return later }, http.StatusOK)
	if _, err := b.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("an expired entry must be refreshed from NVD, got %d calls", *hits)
	}
	fail, _ := nvdWith(t, t.TempDir(), nil, http.StatusServiceUnavailable)
	if _, err := fail.Query(context.Background(), "v", "x", "1.0"); err == nil {
		t.Fatal("expected a failure")
	}
	files, _ := os.ReadDir(fail.CacheDir)
	if len(files) != 0 {
		t.Fatalf("failures must not be cached, found %d files", len(files))
	}
}

// Berkas cache rusak atau milik CPE lain diabaikan, bukan dipercaya.
func TestNVDCacheIgnoresCorruptOrMismatchedFiles(t *testing.T) {
	dir := t.TempDir()
	a, _ := nvdWith(t, dir, nil, http.StatusOK)
	if _, err := a.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	cpe := "cpe:2.3:a:v:p:1.0:*:*:*:*:*:*:*"
	if err := os.WriteFile(a.cachePath(cpe), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, hits := nvdWith(t, dir, nil, http.StatusOK)
	if _, err := b.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("a corrupt cache file must fall back to NVD, got %d calls", *hits)
	}
}

// 404 dari NVD bukan "aman": harus jadi galat, tidak disimpan, dan permintaan tidak boleh memuat isVulnerable.
func TestNVD404IsAnErrorNotAFalseNegative(t *testing.T) {
	var sawParam int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("isVulnerable") {
			atomic.AddInt32(&sawParam, 1)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	dir := t.TempDir()
	n := &NVD{Base: srv.URL, Sleep: func(time.Duration) {}, CacheDir: dir}
	if _, err := n.Query(context.Background(), "v", "p", "1.0"); err == nil {
		t.Fatal("a 404 must not be read as 'no vulnerabilities'")
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatalf("a 404 must never be cached, found %d files", len(files))
	}
	if atomic.LoadInt32(&sawParam) != 0 {
		t.Fatal("the request must not send isVulnerable (NVD answers 404 to it)")
	}
}

// Entri cache versi lama (negatif palsu) tidak dipakai.
func TestNVDCacheIgnoresOldVersionEntries(t *testing.T) {
	dir := t.TempDir()
	a, _ := nvdWith(t, dir, nil, http.StatusOK)
	cpe := "cpe:2.3:a:v:p:1.0:*:*:*:*:*:*:*"
	old := `{"v":1,"cpe":"` + cpe + `","fetched_at":"` + time.Now().UTC().Format(time.RFC3339) + `","vulns":null}`
	if err := os.WriteFile(a.cachePath(cpe), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	b, hits := nvdWith(t, dir, nil, http.StatusOK)
	if _, err := b.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("an old-version cache entry must be re-fetched, got %d calls", *hits)
	}
}
