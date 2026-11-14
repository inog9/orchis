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
		w.WriteHeader(http.StatusNotFound) // tanpa CVE
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
	if got := atomic.LoadInt32(&good); got != 2 {
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
