package scans

import (
	"encoding/json"
	"testing"
)

func TestWebDiscoveryUsesOnlyApprovedOriginPort(t *testing.T) {
	for _, tt := range []struct{ url, host, port string }{
		{"https://juice-shop.herokuapp.com/", "juice-shop.herokuapp.com", "443"},
		{"http://example.test/path", "example.test", "80"},
		{"https://example.test:8443/a", "example.test", "8443"},
		{"http://[2001:db8::1]:8080/", "2001:db8::1", "8080"},
	} {
		t.Run(tt.url, func(t *testing.T) {
			host, raw, err := webDiscovery(tt.url)
			if err != nil {
				t.Fatal(err)
			}
			var params struct {
				Profile, Ports   string
				ServiceDetection bool `json:"service_detection"`
			}
			if err := json.Unmarshal(raw, &params); err != nil {
				t.Fatal(err)
			}
			if host != tt.host || params.Profile != "custom" || params.Ports != tt.port || !params.ServiceDetection {
				t.Fatalf("unexpected discovery: %s %s", host, raw)
			}
		})
	}
	for _, target := range []string{"example.test", "ftp://example.test", "https://example.test:0/", "https://example.test:bad/"} {
		if _, _, err := webDiscovery(target); err == nil {
			t.Errorf("accepted invalid target %s", target)
		}
	}
}
