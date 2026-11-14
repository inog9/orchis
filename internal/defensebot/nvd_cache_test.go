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
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return &NVD{Base: srv.URL, Sleep: func(time.Duration) {}, CacheDir: dir, Now: now}, &hits
}

// Hasil sukses (termasuk "tidak ada CVE") disimpan di disk dan dipakai lagi oleh instans baru (restart worker).
func TestNVDCacheSurvivesRestartAndSkipsTheNetwork(t *testing.T) {
	dir := t.TempDir()
	a, hitsA := nvdWith(t, dir, nil, http.StatusNotFound)
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
	a, _ := nvdWith(t, dir, func() time.Time { return t0 }, http.StatusNotFound)
	if _, err := a.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	later := t0.Add(DefaultNVDCacheTTL + time.Hour)
	b, hits := nvdWith(t, dir, func() time.Time { return later }, http.StatusNotFound)
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
	a, _ := nvdWith(t, dir, nil, http.StatusNotFound)
	if _, err := a.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	cpe := "cpe:2.3:a:v:p:1.0:*:*:*:*:*:*:*"
	if err := os.WriteFile(a.cachePath(cpe), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, hits := nvdWith(t, dir, nil, http.StatusNotFound)
	if _, err := b.Query(context.Background(), "v", "p", "1.0"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("a corrupt cache file must fall back to NVD, got %d calls", *hits)
	}
}
