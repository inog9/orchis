package contract

import (
	"net/netip"
	"testing"
)

func TestScanScopeRejectsExpansionPublicTargetsAndUnauthorizedTargets(t *testing.T) {
	guard := Guard{AllowLoopback: true, Resolve: func(string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("192.168.5.10")}, nil }}
	good := ScanScope{Target: "192.168.5.10", CIDR: "192.168.5.0/24", TestingAuthorized: true, MaxHosts: 256}
	if err := good.Validate(good.Target, guard); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		s      ScanScope
		target string
	}{
		{"target substitution", good, "192.168.5.11"},
		{"unauthorized", ScanScope{Target: good.Target, CIDR: good.CIDR, MaxHosts: 256}, good.Target},
		{"public", ScanScope{Target: "8.8.8.8", TestingAuthorized: true}, "8.8.8.8"},
		{"hostname public", ScanScope{Target: "https://example.test", TestingAuthorized: true}, "https://example.test"},
		{"outside network", ScanScope{Target: "192.168.6.10", CIDR: good.CIDR, TestingAuthorized: true, MaxHosts: 256}, "192.168.6.10"},
		{"oversized network", ScanScope{Target: good.Target, CIDR: "192.168.4.0/23", TestingAuthorized: true, MaxHosts: 256}, good.Target},
		{"default route", ScanScope{Target: good.Target, CIDR: "0.0.0.0/0", TestingAuthorized: true, MaxHosts: 4096}, good.Target},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.s.Validate(tt.target, guard); err == nil {
				t.Fatal("unsafe scan scope accepted")
			}
		})
	}
	public := ScanScope{Target: "8.8.8.8", TestingAuthorized: true, InternetFacing: true}
	if err := public.Validate(public.Target, guard); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"192.168.0.0/23", "::/0", "fc00::/24", "169.254.169.254/32", "224.0.0.0/24", "fd00:ec2::254/128", "127.0.0.0/24"} {
		if _, err := BoundedNetwork(raw, 256, Guard{}); err == nil {
			t.Fatalf("unsafe network accepted: %s", raw)
		}
	}
	for _, raw := range []string{"192.168.0.0/24", "fd00:abcd::/120"} {
		if _, err := BoundedNetwork(raw, 256, guard); err != nil {
			t.Fatalf("bounded family: %s %v", raw, err)
		}
	}
}

func FuzzBoundedNetworkNeverExceedsAddressLimit(f *testing.F) {
	for _, s := range []string{"192.168.0.0/24", "fd00:abcd::/120", "0.0.0.0/0", "192.168.1.7/24"} {
		f.Add(s, uint16(256))
	}
	f.Fuzz(func(t *testing.T, raw string, limit uint16) {
		max := int(limit)
		p, err := BoundedNetwork(raw, max, Guard{})
		if err != nil {
			return
		}
		n := 0
		for a := p.Addr(); p.Contains(a); a = a.Next() {
			n++
			if n > max {
				t.Fatal("host limit bypass")
			}
			if !a.Next().IsValid() {
				break
			}
		}
	})
}
