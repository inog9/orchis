package server_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAgentAddressNeedsAdministratorConfirmationAtRunAndClaim(t *testing.T) {
	f := newScanFixture(t, true)
	ctx := context.Background()
	queued := f.run(t)
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE assets SET source='agent' WHERE id=$1`, f.asset); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE asset_addresses SET scan_approved=false WHERE asset_id=$1`, f.asset); err != nil {
		t.Fatal(err)
	}
	r := f.a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.analyst)
	if r.Status != 422 {
		t.Fatalf("unconfirmed agent address allowed at Run: %d %s", r.Status, r.Body)
	}
	r = f.inj("POST", "executions/"+queued.ID+"/claim", nil, f.token)
	if r.Status != 409 {
		t.Fatalf("unconfirmed frozen agent address allowed at Claim: %d %s", r.Status, r.Body)
	}
	ended := f.exec(t, queued.ID)
	if ended.Outcome != "error" || !strings.Contains(ended.Error, "administrator confirmation") {
		t.Fatalf("missing explicit scope refusal: %+v", ended)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE asset_addresses SET scan_approved=true WHERE asset_id=$1`, f.asset); err != nil {
		t.Fatal(err)
	}
	approved := f.run(t)
	if r = f.inj("POST", "executions/"+approved.ID+"/claim", nil, f.token); r.Status != 200 {
		t.Fatalf("confirmed admin address rejected: %d %s", r.Status, r.Body)
	}
}

func TestLegacyAutomaticAgentHostnameMigrationPreservesDataAndRevokesQueue(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	enrolled, _ := f.enroll(t, "legacy-auto.internal")
	edited, _ := f.enroll(t, "admin-edited.internal")
	var automaticAsset, editedAsset string
	for _, item := range []struct {
		agent string
		dest  *string
	}{{enrolled.AgentID, &automaticAsset}, {edited.AgentID, &editedAsset}} {
		if err := f.a.env.Owner.QueryRow(ctx, `SELECT asset_id FROM agents WHERE id=$1`, item.agent).Scan(item.dest); err != nil {
			t.Fatal(err)
		}
	}
	queued := f.run(t)
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE executions SET target_asset_id=$2,target_address='http://legacy-auto.internal:80' WHERE id=$1`, queued.ID, automaticAsset); err != nil {
		t.Fatal(err)
	}
	// Bentuk persis data sebelum migrasi29; database ini hanya milik tes.
	if _, err := f.a.env.Owner.Exec(ctx, `ALTER TABLE asset_addresses DROP COLUMN scan_approved`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `INSERT INTO asset_addresses(id,tenant_id,asset_id,kind,value,created_at) SELECT gen_random_uuid(),a.tenant_id,a.id,'hostname',g.hostname,a.created_at+CASE WHEN g.id=$2 THEN interval '1 second' ELSE interval '0' END FROM assets a JOIN agents g ON g.asset_id=a.id WHERE g.id IN($1,$2)`, enrolled.AgentID, edited.AgentID); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/00029_confirmed_agent_addresses.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.env.Owner.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	var automatic, manual bool
	var n int
	if err = f.a.env.Owner.QueryRow(ctx, `SELECT scan_approved FROM asset_addresses WHERE asset_id=$1`, automaticAsset).Scan(&automatic); err != nil {
		t.Fatal(err)
	}
	if err = f.a.env.Owner.QueryRow(ctx, `SELECT scan_approved FROM asset_addresses WHERE asset_id=$1`, editedAsset).Scan(&manual); err != nil {
		t.Fatal(err)
	}
	if automatic || !manual {
		t.Fatalf("legacy auto=%v admin edit=%v", automatic, manual)
	}
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM asset_addresses WHERE asset_id IN($1,$2)`, automaticAsset, editedAsset).Scan(&n)
	if n != 2 {
		t.Fatal("historical addresses were deleted")
	}
	ended := f.exec(t, queued.ID)
	if ended.Outcome != "error" || !strings.Contains(ended.Error, "administrator confirmation") {
		t.Fatalf("legacy queue survived: %+v", ended)
	}
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE resource_id=$1 AND action='execution.scope_revoked'`, queued.ID).Scan(&n)
	if n != 1 {
		t.Fatal("migration revocation audit missing")
	}
}

func TestFrozenChainAgentAddressIsRecheckedBeforeNextStep(t *testing.T) {
	c := newChainFixture(t)
	ctx := context.Background()
	c.mustReady(t, []map[string]any{step("first", "nmap.tcp_connect", "after_parents", nil), step("next", "nmap.tcp_connect", "after_parents", nil)}, [][2]string{{"first", "next"}})
	sim := c.launch(t)
	var id string
	if err := c.a.env.Owner.QueryRow(ctx, `SELECT e.id FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key='first'`, sim.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if r := c.inj("POST", "executions/"+id+"/claim", nil, c.token); r.Status != 200 {
		t.Fatalf("initial claim: %d %s", r.Status, r.Body)
	}
	if _, err := c.a.env.Owner.Exec(ctx, `UPDATE simulation_targets SET addresses='[{"kind":"ip","value":"127.0.0.1"}]' WHERE simulation_id=$1`, sim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.a.env.Owner.Exec(ctx, `UPDATE assets SET source='agent' WHERE id=$1`, c.asset); err != nil {
		t.Fatal(err)
	}
	if _, err := c.a.env.Owner.Exec(ctx, `UPDATE asset_addresses SET scan_approved=false WHERE asset_id=$1`, c.asset); err != nil {
		t.Fatal(err)
	}
	if r := c.inj("POST", "executions/"+id+"/result", okResult, c.token); r.Status != 204 {
		t.Fatalf("first result: %d %s", r.Status, r.Body)
	}
	var state, reason string
	if err := c.a.env.Owner.QueryRow(ctx, `SELECT e.state,COALESCE(e.error,'') FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key='next'`, sim.ID).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	if state != "ended" || !strings.Contains(reason, "administrator confirmation") {
		t.Fatalf("unconfirmed frozen chain target reached next step: %s %s", state, reason)
	}
}

func TestAuthorizedAssetAddressChangeRequiresAuthorizer(t *testing.T) {
	f := newScanFixture(t, true)
	body := map[string]any{"version": 1, "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.2"}}}
	r := f.a.do("PATCH", "/api/v1/assets/"+f.asset, body, f.analyst)
	if r.Status != 403 {
		t.Fatalf("non-authorizer replaced approved target: %d %s", r.Status, r.Body)
	}
	r = f.a.do("PATCH", "/api/v1/assets/"+f.asset, body, f.admin)
	if r.Status != 200 {
		t.Fatalf("admin address review failed: %d %s", r.Status, r.Body)
	}
}
