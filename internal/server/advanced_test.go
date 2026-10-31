package server_test

import (
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestAdvancedGraphMapsPerAssetAndDryRunDoesNotExecute(t *testing.T) {
	c := newChainFixture(t)
	second := mkAsset(t, c.a, c.admin, map[string]any{"category": "host", "name": "second", "testing_authorized": true, "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.2"}}})
	if r := c.targets(t, []string{c.asset, second}, nil); r.Status != 200 {
		t.Fatalf("targets %d %s", r.Status, r.Body)
	}
	steps := []map[string]any{step("scan", "nmap.tcp_connect", "after_parents", nil), step("yes", "nmap.tcp_connect", "after_parents", nil), step("no", "nmap.tcp_connect", "after_parents", nil)}
	condition := map[string]any{"op": "GT", "field": "finding_count", "value": 0}
	edges := []map[string]any{{"parent": "scan", "child": "yes", "condition": condition, "mapper": map[string]string{"source": "open_ports", "destination": "ports"}}, {"parent": "scan", "child": "no", "condition": condition, "branch": "else"}}
	r := c.a.do("PUT", "/api/v1/scenarios/"+c.scenario+"/graph", map[string]any{"version": c.version, "steps": steps, "edges": edges}, c.analyst)
	if r.Status != 200 {
		t.Fatalf("graph %d %s", r.Status, r.Body)
	}
	sim := c.launch(t)
	type row struct{ id, asset string }
	list := []row{}
	rows, e := c.a.env.Owner.Query(context.Background(), `SELECT e.id,e.target_asset_id FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key='scan'`, sim.ID)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var x row
		rows.Scan(&x.id, &x.asset)
		list = append(list, x)
	}
	rows.Close()
	if len(list) != 2 {
		t.Fatalf("root execution count %d", len(list))
	}
	for _, x := range list {
		if r = c.inj("POST", "executions/"+x.id+"/claim", nil, c.token); r.Status != 200 {
			t.Fatalf("claim %d %s", r.Status, r.Body)
		}
		findings := []map[string]any{}
		if x.asset == c.asset {
			findings = append(findings, map[string]any{"type": "open_port", "value": "tcp/80", "title": "HTTP", "severity": "info"})
		}
		if r = c.inj("POST", "executions/"+x.id+"/result", map[string]any{"exit_code": 0, "findings": findings}, c.token); r.Status != 204 {
			t.Fatalf("result %d %s", r.Status, r.Body)
		}
	}
	var yesAsset, ports, noAsset string
	e = c.a.env.Owner.QueryRow(context.Background(), `SELECT e.target_asset_id,e.params->>'ports' FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key='yes'`, sim.ID).Scan(&yesAsset, &ports)
	if e != nil || yesAsset != c.asset || ports != "80" {
		t.Fatalf("mapper crossed assets: %s %s %v", yesAsset, ports, e)
	}
	e = c.a.env.Owner.QueryRow(context.Background(), `SELECT e.target_asset_id FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key='no'`, sim.ID).Scan(&noAsset)
	if e != nil || noAsset != second {
		t.Fatalf("else crossed assets: %s %v", noAsset, e)
	}
	var before, after int
	c.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE simulation_id=$1`, sim.ID).Scan(&before)
	r = c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/dry-run", nil, c.viewer)
	if r.Status != 200 {
		t.Fatalf("dry run %d %s", r.Status, r.Body)
	}
	c.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE simulation_id=$1`, sim.ID).Scan(&after)
	if before != after {
		t.Fatal("dry run executed checks")
	}
	var count int
	c.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM simulation_decisions WHERE simulation_id=$1 AND reason<>''`, uuid.MustParse(sim.ID)).Scan(&count)
	if count < 4 {
		t.Fatalf("missing decisions %d", count)
	}
}
