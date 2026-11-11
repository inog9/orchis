package injector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/contract"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebEndpointDistinguishesUnavailableFromNoFindings(t *testing.T) {
	guard := contract.Guard{AllowLoopback: true}
	for _, code := range []int{200, 401, 404, 405, 500} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		if err := checkWebEndpoint(context.Background(), s.URL, guard); err != nil {
			t.Fatalf("HTTP %d must be reachable: %v", code, err)
		}
		s.Close()
		if err := checkWebEndpoint(context.Background(), s.URL, guard); err == nil {
			t.Fatal("closed endpoint must fail, not yield a clean result")
		}
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer s.Close()
	if err := checkWebEndpoint(context.Background(), s.URL, guard); err != nil {
		t.Fatalf("self-signed test service: %v", err)
	}
	if err := checkWebEndpoint(context.Background(), s.URL, contract.Guard{}); err == nil {
		t.Fatal("scope guard must still block loopback")
	}
}

func TestWebEndpointRetriesTransientFailure(t *testing.T) {
	var attempts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer s.Close()
	if err := checkWebEndpoint(context.Background(), s.URL, contract.Guard{AllowLoopback: true}); err != nil {
		t.Fatalf("transient failure did not recover: %v", err)
	}
	if attempts.Load() < 2 {
		t.Fatal("did not retry")
	}
}
func TestWebEndpointDoesNotFollowRedirectOrProxy(t *testing.T) {
	var hits atomic.Int32
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer forbidden.Close()
	t.Setenv("HTTP_PROXY", forbidden.URL)
	t.Setenv("HTTPS_PROXY", forbidden.URL)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, forbidden.URL, 302) }))
	defer source.Close()
	if err := checkWebEndpoint(context.Background(), source.URL, contract.Guard{AllowLoopback: true}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatal("probe followed redirect or used ambient proxy")
	}
	for _, target := range []string{"http://169.254.169.254/", "http://user:pass@127.0.0.1/", "file:///tmp/data"} {
		if err := checkWebEndpoint(context.Background(), target, contract.Guard{AllowLoopback: true}); err == nil {
			t.Fatal("unsafe target accepted")
		}
	}
}
func TestWebEndpointHonorsCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := checkWebEndpoint(ctx, s.URL, contract.Guard{AllowLoopback: true}); err == nil {
		t.Fatal("silent endpoint must fail")
	}
	if time.Since(started) > time.Second {
		t.Fatal("probe ignored deadline")
	}
}
