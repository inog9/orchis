package scanscope

import (
	"orchis-go/internal/contract"
	"testing"
)

func TestReportedRangesRequireARealNetworkAndCannotExpandAgentScope(t *testing.T) {
	rows := []map[string]string{{"address": "192.168.42.10", "mask": "255.255.255.0"}, {"address": "0.0.0.0", "mask": "0"}, {"address": "169.254.169.254", "mask": "32"}, {"address": "192.168.1.1", "mask": "255.0.255.0"}, {"address": "fd00:abcd::1", "mask": "120"}}
	networks := ReportedNetworks(rows, contract.Guard{})
	if len(networks) != 2 {
		t.Fatalf("untrusted ranges: %v", networks)
	}
	for _, raw := range []string{"192.168.42.0/24", "192.168.42.128/25", "fd00:abcd::/120"} {
		if !ContainsReported(raw, rows, contract.Guard{}) {
			t.Fatalf("reported subset refused: %s", raw)
		}
	}
	for _, raw := range []string{"192.168.0.0/16", "8.8.8.0/24", "0.0.0.0/0", "fd00::/16", "192.168.42.1/24"} {
		if ContainsReported(raw, rows, contract.Guard{}) {
			t.Fatalf("unreported network accepted: %s", raw)
		}
	}
}
