package auth

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestTrustedProxy(t *testing.T) {
	for _, tc := range []struct{ name, peer, xff, want string }{
		{"direct spoof", "198.51.100.2:443", "192.0.2.5", "198.51.100.2"},
		{"trusted", "10.0.0.5:80", "192.0.2.5", "192.0.2.5"},
		{"left spoof", "10.0.0.5:80", "192.0.2.7,198.51.100.2", "198.51.100.2"},
		{"trusted chain", "10.0.0.5:80", "192.0.2.7,10.0.0.6", "192.0.2.7"},
		{"invalid closest", "10.0.0.5:80", "192.0.2.7,evil", "10.0.0.5"},
		{"port injection", "10.0.0.5:80", "192.0.2.7:99", "10.0.0.5"},
		{"empty", "10.0.0.5:80", "", "10.0.0.5"},
		{"oversize", "10.0.0.5:80", strings.Repeat("1", 4097), "10.0.0.5"},
		{"mapped IPv4", "[::ffff:10.0.0.5]:80", "2001:db8::1", "2001:db8::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.peer
			req.Header.Set("X-Forwarded-For", tc.xff)
			h := TrustedProxy([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := ClientIP(r); got != tc.want {
					t.Fatalf("got %s want %s", got, tc.want)
				}
			}))
			h.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
