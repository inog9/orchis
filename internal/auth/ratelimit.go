package auth

import (
	"net/http"
	"orchis-go/internal/httpx"
	"sync"
	"time"
)

type publicBucket struct {
	until time.Time
	count int
}

// PublicLimiter bounds credential work before body decoding. It stores only
// normalized client IP and endpoint group, never credentials or reset tokens.
type PublicLimiter struct {
	mu        sync.Mutex
	buckets   map[string]publicBucket
	now       func() time.Time
	nextSweep time.Time
	capacity  int
}

func NewPublicLimiter() *PublicLimiter {
	return &PublicLimiter{buckets: map[string]publicBucket{}, now: time.Now, capacity: 4096}
}
func (l *PublicLimiter) allow(key string, limit int) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.nextSweep) {
		for k, b := range l.buckets {
			if !now.Before(b.until) {
				delete(l.buckets, k)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}
	b, exists := l.buckets[key]
	if !exists || !now.Before(b.until) {
		if !exists && len(l.buckets) >= l.capacity {
			return false, 60
		}
		b = publicBucket{until: now.Add(time.Minute)}
	}
	retry := int(b.until.Sub(now).Seconds()) + 1
	if b.count >= limit {
		return false, retry
	}
	b.count++
	l.buckets[key] = b
	return true, 0
}
func (l *PublicLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group, limit := "", 10
		if r.Method == http.MethodPost {
			switch r.URL.Path {
			case "/api/v1/auth/login":
				group = "login"
			case "/api/v1/auth/forgot", "/api/v1/auth/reset", "/api/v1/auth/reset/check":
				group = "reset"
			case "/api/v1/agent/enroll":
				group = "enroll"
				limit = 20
			}
		}
		if group != "" {
			if ok, retry := l.allow(ClientIP(r)+"/"+group, limit); !ok {
				p := httpx.NewProblem(429, "Too many requests", "Try again after the indicated delay.")
				p.RetryAfter = retry
				httpx.Write(w, r, p)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
