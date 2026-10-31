// Package telemetry exposes bounded, authenticated operational metrics without user labels.
package telemetry

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	mu     sync.Mutex
	values map[string]float64
}

func New() *Registry { return &Registry{values: map[string]float64{}} }

// All metric names and labels must be compile-time constants or fixed enums.
func (m *Registry) Set(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[name] = value
}
func (m *Registry) Add(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[name] += value
}
func (m *Registry) Heartbeat() {
	m.Set("orchis_worker_last_success_timestamp_seconds", float64(time.Now().Unix()))
}
func (m *Registry) Handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/metrics" || r.Method != "GET" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.mu.Lock()
		defer m.mu.Unlock()
		names := make([]string, 0, len(m.values))
		for name := range m.values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(w, "%s %g\n", name, m.values[name])
		}
	})
}

// Empty token disables the listener for development. Production setup supplies it.
func Start(ctx context.Context) (*Registry, error) {
	m := New()
	token := os.Getenv("ORCHIS_METRICS_TOKEN")
	if token == "" {
		return m, nil
	}
	if len(token) < 32 {
		return nil, errors.New("metrics token must have at least 32 bytes")
	}
	addr := os.Getenv("ORCHIS_METRICS_ADDR")
	if addr == "" {
		addr = ":9101"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.New("cannot listen for internal metrics")
	}
	server := &http.Server{Handler: m.Handler(token), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 4096}
	m.Set("orchis_process_start_timestamp_seconds", float64(time.Now().Unix()))
	m.Set("orchis_worker_last_success_timestamp_seconds", 0)
	go server.Serve(listener)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	return m, nil
}
func group(path string) string {
	for _, p := range []string{"auth", "assets", "organizations", "assessments", "reports", "agents", "agent", "injectors", "injector", "ai-worker", "ai", "findings", "home", "audit"} {
		if path == "/api/v1/"+p || strings.HasPrefix(path, "/api/v1/"+p+"/") {
			return p
		}
	}
	if strings.HasPrefix(path, "/api/") {
		return "api_other"
	}
	return "web"
}

type response struct {
	http.ResponseWriter
	status int
}

func (w *response) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *response) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (m *Registry) HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &response{ResponseWriter: w}
		defer func() {
			code := rw.status
			if code == 0 {
				code = 200
			}
			class := code / 100
			if class < 1 || class > 5 {
				class = 5
			}
			label := fmt.Sprintf("{route=%q,status_class=%q}", group(r.URL.Path), fmt.Sprintf("%dxx", class))
			elapsed := time.Since(start).Seconds()
			m.Add("orchis_http_requests_total"+label, 1)
			m.Add("orchis_http_request_duration_seconds_sum"+label, elapsed)
			m.Add("orchis_http_request_duration_seconds_count"+label, 1)
		}()
		next.ServeHTTP(rw, r)
	})
}
