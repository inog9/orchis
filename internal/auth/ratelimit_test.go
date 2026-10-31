package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterConcurrentAndExpiry(t *testing.T) {
	l := NewPublicLimiter()
	now := time.Unix(100, 0)
	l.now = func() time.Time { return now }
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if ok, _ := l.allow("client/login", 10); ok {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("allowed %d", allowed.Load())
	}
	now = now.Add(time.Minute)
	if ok, _ := l.allow("client/login", 10); !ok {
		t.Fatal("did not expire")
	}
	l.capacity = 1
	if ok, _ := l.allow("other/login", 10); ok {
		t.Fatal("unbounded client registry")
	}
}
func TestLimiterEndpointIsolation(t *testing.T) {
	l := NewPublicLimiter()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := l.Middleware(next)
	for i := 0; i < 11; i++ {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.RemoteAddr = "192.0.2.1:123"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if i < 10 && w.Code != 204 {
			t.Fatal(w.Code)
		}
		if i == 10 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatal("missing bounded 429 retry")
		}
	}
	for _, path := range []string{"/api/v1/agent/enroll", "/api/v1/auth/reset/check", "/api/healthz"} {
		r := httptest.NewRequest("POST", path, nil)
		r.RemoteAddr = "192.0.2.1:123"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal("endpoint isolation failed")
		}
	}
}

func TestForgotAndResetSharePublicBudget(t *testing.T) {
	l := NewPublicLimiter()
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, path := range []string{"/api/v1/auth/forgot", "/api/v1/auth/reset/check", "/api/v1/auth/reset"} {
		l = NewPublicLimiter()
		h = l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
		for range 10 {
			r := httptest.NewRequest("POST", path, nil)
			r.RemoteAddr = "192.0.2.8:1"
			h.ServeHTTP(httptest.NewRecorder(), r)
		}
		r := httptest.NewRequest("POST", "/api/v1/auth/forgot", nil)
		r.RemoteAddr = "192.0.2.8:2"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 429 {
			t.Fatal("public reset budget bypassed", path, w.Code)
		}
	}
}
