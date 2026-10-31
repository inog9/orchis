package auth

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPKey struct{}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String()
	}
	return "unknown"
}

// TrustedProxy accepts XFF only from an explicit peer and walks from the closest
// proxy toward the client. A spoofed leftmost value never overrides an untrusted hop.
func TrustedProxy(prefixes []netip.Prefix, next http.Handler) http.Handler {
	trusted := func(ip netip.Addr) bool {
		for _, p := range prefixes {
			if p.Contains(ip.Unmap()) {
				return true
			}
		}
		return false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := remoteIP(r)
		peer, err := netip.ParseAddr(raw)
		if err == nil && trusted(peer) {
			header := r.Header.Values("X-Forwarded-For")
			text := strings.Join(header, ",")
			if len(text) <= 4096 {
				hops := strings.Split(text, ",")
				if len(hops) <= 32 {
					candidate := peer
					valid := true
					for i := len(hops) - 1; i >= 0 && trusted(candidate); i-- {
						ip, e := netip.ParseAddr(strings.TrimSpace(hops[i]))
						if e != nil {
							valid = false
							break
						}
						candidate = ip.Unmap()
					}
					if valid {
						raw = candidate.String()
					}
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, raw)))
	})
}
