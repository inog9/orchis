package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAuthAndBoundedRoutes(t *testing.T) {
	m := New()
	token := strings.Repeat("x", 32)
	for _, path := range []string{"/api/v1/assets/secret-asset?token=secret", "/api/v1/assets/other", "/arbitrary/secret"} {
		m.HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	for _, auth := range []string{"", "Bearer wrong"} {
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		m.Handler(token).ServeHTTP(w, r)
		if w.Code != 401 || w.Body.Len() != 0 {
			t.Fatal("metrics exposed")
		}
	}
	r := httptest.NewRequest("GET", "/metrics", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	m.Handler(token).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `orchis_http_requests_total{route="assets",status_class="2xx"} 2`) {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "other") {
		t.Fatal("unbounded or secret label")
	}
}
