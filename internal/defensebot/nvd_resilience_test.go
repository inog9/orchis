package defensebot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Satu produk yang terus gagal (NVD 503) tidak boleh menggugurkan pemeriksaan produk lain.
func TestNVDFailureOnOneProductDoesNotAbortTheOthers(t *testing.T) {
	var good int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "badvendor") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		atomic.AddInt32(&good, 1)
		_, _ = w.Write([]byte(`{"totalResults":0,"vulnerabilities":[]}`)) // tanpa CVE
	}))
	defer srv.Close()
	d := Deps{NVD: &NVD{Base: srv.URL, Sleep: func(time.Duration) {}}}
	p := &plan{CPE: []cpeComp{
		{Vendor: "badvendor", Product: "x", Name: "Bad", Version: "1.0"},
		{Vendor: "goodvendor", Product: "a", Name: "A", Version: "1.0"},
		{Vendor: "goodvendor", Product: "b", Name: "B", Version: "2.0"},
	}}
	sum := &Summary{}
	nvdCandidates(context.Background(), p, d, sum)
	if got := atomic.LoadInt32(&good); got < 2 {
		t.Fatalf("expected both healthy products to be queried, got %d", got)
	}
	limiting := false
	for _, n := range sum.Notes {
		if n.Code == "source_down" && n.Limiting {
			limiting = true
		}
	}
	if !limiting {
		t.Fatal("the failed product must still mark the result as limited")
	}
}

// Produk yang gagal sesaat (503) dicoba ulang di akhir; bila berhasil, hasil tidak ditandai terbatas.
func TestNVDTransientFailureIsRetriedAndDoesNotLimitTheResult(t *testing.T) {
	var flakyHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "flaky") && atomic.AddInt32(&flakyHits, 1) <= 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"totalResults":0,"vulnerabilities":[]}`))
	}))
	defer srv.Close()
	d := Deps{NVD: &NVD{Base: srv.URL, Sleep: func(time.Duration) {}}}
	p := &plan{CPE: []cpeComp{{Vendor: "flaky", Product: "x", Name: "Flaky", Version: "1.0"}, {Vendor: "steady", Product: "y", Name: "Steady", Version: "2.0"}}}
	sum := &Summary{}
	nvdCandidates(context.Background(), p, d, sum)
	for _, n := range sum.Notes {
		if n.Code == "source_down" {
			t.Fatalf("a transient 503 that recovers on retry must not mark the scan as limited: %+v", sum.Notes)
		}
	}
}
