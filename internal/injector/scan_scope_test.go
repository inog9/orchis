package injector

import (
	"context"
	"encoding/json"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"strings"
	"testing"
)

func TestInjectorScanScopeRefusesTargetSubstitutionBeforeLaunchingTool(t *testing.T) {
	w := Worker{Kind: "nmap", Binary: "/nonexistent-orchis-scope-proof-tool", Guard: contract.Guard{AllowLoopback: true}}
	proof := &contract.ScanScope{Target: "192.168.50.1", CIDR: "192.168.50.0/24", TestingAuthorized: true, MaxHosts: 256}
	spec := execution.ClaimSpec{ContractKey: contract.KeyNmapTCP, ContractVersion: 1, Target: "127.0.0.1", Params: json.RawMessage(`{"profile":"top100"}`), ScanScope: proof}
	res := w.execute(context.Background(), &spec)
	if !strings.Contains(res.Error, "refused by the injector scan scope") {
		t.Fatalf("injector accepted scope substitution: %s", res.Error)
	}
}
