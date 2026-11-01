package simulation

import (
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"strings"
	"testing"
)

func TestMapperOnlyConsumesDirectParentAndAllowedValues(t *testing.T) {
	s := stepRow{Key: "child", ContractKey: contract.KeyNmapTCP, Params: []byte(`{"profile":"top100"}`)}
	edges := []chain.Edge{{Parent: "parent", Child: "child", Mapper: &chain.Mapper{Source: "open_ports", Destination: "ports"}}}
	found := []chain.Found{{Step: "other", Type: "open_port", Value: "tcp/22"}, {Step: "parent", Type: "open_port", Value: "tcp/80"}, {Step: "parent", Type: "open_port", Value: "80;id"}}
	mapped, _, err := mapInput(s, target{}, edges, found)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mapped.Params), `"ports":"80"`) {
		t.Fatalf("mapping %s", mapped.Params)
	}
	if _, _, e := mapInput(s, target{}, edges, found[:1]); e == nil {
		t.Fatal("used findings from unrelated parent")
	}
	s.ContractKey = contract.KeyNucleiVS
	edges[0].Mapper = &chain.Mapper{Source: "web_urls", Destination: "target"}
	s.Params = []byte(`{"severity":["high"]}`)
	_, target, e := mapInput(s, target{Addrs: []contract.Address{{Kind: "ip", Value: "192.0.2.10"}}}, edges, found)
	if e != nil {
		t.Fatal(e)
	}
	if len(target.Addrs) != 1 || target.Addrs[0].Value != "http://192.0.2.10:80" {
		t.Fatalf("target broadened %+v", target)
	}
}
func TestWebMapperUsesObservedParentHostAndRejectsScopeExpansion(t *testing.T) {
	s := stepRow{Key: "child", ContractKey: contract.KeyNucleiVS, Params: []byte(`{"severity":["high"]}`)}
	edges := []chain.Edge{{Parent: "parent", Child: "child", Mapper: &chain.Mapper{Source: "web_urls", Destination: "target"}}}
	asset := target{Addrs: []contract.Address{{Kind: "ip", Value: "192.0.2.10"}, {Kind: "url", Value: "https://another.example.test"}}}
	found := []chain.Found{{Step: "parent", Type: "open_port", Value: "tcp/80", Target: "192.0.2.10"}}
	_, mapped, e := mapInput(s, asset, edges, found)
	if e != nil || mapped.Addrs[0].Value != "http://192.0.2.10:80" {
		t.Fatalf("mapper changed observed host %+v %v", mapped, e)
	}
	found[0].Target = "198.51.100.20"
	if _, _, e = mapInput(s, asset, edges, found); e == nil {
		t.Fatal("accepted a host outside the frozen asset scope")
	}
}

func TestSoftwareMapperUsesFrozenTagTableOnly(t *testing.T) {
	s := stepRow{Key: "child", ContractKey: contract.KeyNucleiVS, Params: []byte(`{"severity":["high"]}`)}
	edges := []chain.Edge{{Parent: "parent", Child: "child", Mapper: &chain.Mapper{Source: "installed_software", Destination: "tags"}}}
	found := []chain.Found{
		{Step: "parent", Type: "service", Value: "osquery:package:nginx"},
		{Step: "parent", Type: "service", Value: "osquery:package:apache2"},
		{Step: "parent", Type: "service", Value: "osquery:package:redis-server"},
		{Step: "parent", Type: "service", Value: "osquery:package:evil;id"},
		{Step: "parent", Type: "text", Value: "osquery:package:httpd"},
		{Step: "other", Type: "service", Value: "osquery:package:httpd"},
	}
	mapped, _, err := mapInput(s, target{}, edges, found)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mapped.Params), `"tags":["apache","nginx"]`) {
		t.Fatalf("tags %s", mapped.Params)
	}
	if _, _, e := mapInput(s, target{}, edges, found[2:5]); e == nil {
		t.Fatal("unsupported or non-service or raw names produced a mapping")
	}
	if _, _, e := mapInput(s, target{}, edges, found[5:]); e == nil {
		t.Fatal("used software from an unrelated parent")
	}
	s.ContractKey = contract.KeyNmapTCP
	if _, _, e := mapInput(s, target{}, edges, found); e == nil {
		t.Fatal("software mapping accepted for a non-nuclei step")
	}
}
