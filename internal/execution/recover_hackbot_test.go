package execution_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
	"orchis-go/internal/simulation"
	"orchis-go/internal/testutil"
)

func TestHackbotRecoveryPreservesHistoryAndSameURLFindings(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	tenant := env.TenantA
	org, actor, asset, scan, integration := ids.New(), ids.New(), ids.New(), ids.New(), uuid.Nil
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := env.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'Recovery fixture','recovery-fixture')`, org, tenant)
	must(`INSERT INTO users(id,tenant_id,email,display_name,role_id) SELECT $1,$2,'recover@example.test','Fixture',id FROM roles WHERE tenant_id=$2 AND key='admin'`, actor, tenant)
	must(`INSERT INTO assets(id,tenant_id,organization_id,name,category) VALUES($1,$2,$3,'Fixture asset','host')`, asset, tenant, org)
	must(`INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,status,created_by,ended_at,is_scan) VALUES($1,$2,$3,'Fixture scan','quick','source','source','incomplete',$4,now(),true)`, scan, tenant, org, actor)
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM integrations WHERE tenant_id=$1 AND key='hackbot'`, tenant).Scan(&integration); err != nil {
		t.Fatal(err)
	}
	var original uuid.UUID
	for _, pair := range []struct{ tool, key string }{{"nmap", contract.KeyNmapTCP}, {"nuclei", contract.KeyNucleiVS}, {"hackbot", contract.KeyHackbot}} {
		sim, step, ex, j := ids.New(), ids.New(), ids.New(), ids.New()
		var cid uuid.UUID
		var version int
		if err := env.Owner.QueryRow(ctx, `SELECT id,version FROM contracts WHERE tenant_id=$1 AND key=$2`, tenant, pair.key).Scan(&cid, &version); err != nil {
			t.Fatal(err)
		}
		must(`INSERT INTO simulations(id,tenant_id,name,status,started_at,ended_at,launched_by) VALUES($1,$2,$3,'finished',now(),now(),$4)`, sim, tenant, pair.tool, actor)
		must(`INSERT INTO simulation_steps(id,tenant_id,simulation_id,key,title,contract_id,contract_version,trigger_mode) VALUES($1,$2,$3,$4,$4,$5,$6,'after_parents')`, step, tenant, sim, pair.tool, cid, version)
		must(`INSERT INTO simulation_targets(tenant_id,simulation_id,asset_id,addresses) VALUES($1,$2,$3,'[]')`, tenant, sim, asset)
		outcome, status := "success", "finished"
		if pair.tool == "hackbot" {
			outcome, status = "error", "failed"
			original = ex
		}
		must(`INSERT INTO executions(id,tenant_id,simulation_id,simulation_step_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,state,outcome,input_hash,started_at,ended_at) VALUES($1,$2,$3,$4,$5,$6,$7,'https://example.test/','injector',$8,'ended',$9,$10,now(),now())`, ex, tenant, sim, step, cid, version, asset, integration, outcome, []byte(pair.tool))
		must(`INSERT INTO scan_jobs(id,tenant_id,assessment_id,control_asset_id,asset_id,name,target,addresses,tool,contract_id,contract_version,params,scope,simulation_id,status,ended_at) VALUES($1,$2,$3,$4,$4,'Fixture','https://example.test/','[]',$5,$6,$7,'{}','{}',$8,$9,now())`, j, tenant, scan, asset, pair.tool, cid, version, sim, status)
		if pair.tool == "nmap" {
			finding := ids.New()
			must(`INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details) VALUES($1,$2,$3,'open_port','tcp/443',$4,'HTTPS','info','{}')`, finding, tenant, asset, []byte("fixture-port"))
			must(`INSERT INTO finding_occurrences(id,tenant_id,finding_id,execution_id,simulation_id,details,finding_snapshot) VALUES($1,$2,$3,$4,$5,'{}','{"type":"open_port","value":"tcp/443","title":"HTTPS","severity":"info"}')`, ids.New(), tenant, finding, ex, sim)
		}
	}
	report := ids.New()
	must(`INSERT INTO reports(id,tenant_id,organization_id,assessment_id,snapshot) VALUES($1,$2,$3,$4,'{"original":true}')`, report, tenant, org, scan)
	detail := json.RawMessage(`{"verdict":"VALIDATED","evidence":"example"}`)
	res := hackbot.Result{Schema: 1, SessionID: original.String(), Disposition: "complete", Phase: "report", Summary: "Reviewed", Report: "Report", Validation: "Ledger", Findings: []hackbot.Finding{{ID: "1", Title: "Issue A", Value: "https://example.test/", Severity: "low", Details: detail}, {ID: "2", Title: "Issue B", Value: "https://example.test/", Severity: "high", Details: detail}}}
	rt := execution.Runtime{DB: env.App, Tenant: tenant}
	in := &execution.Injector{ID: integration}
	if _, err := rt.RecoverHackbot(ctx, &execution.Injector{ID: ids.New()}, original, res); err == nil {
		t.Fatal("wrong integration accepted")
	}
	recovered, err := rt.RecoverHackbot(ctx, in, original, res)
	if err != nil {
		t.Fatal(err)
	}
	again, err := rt.RecoverHackbot(ctx, in, original, res)
	if err != nil || again != recovered {
		t.Fatalf("replay: %s %v", again, err)
	}
	var n int
	if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM finding_occurrences o JOIN scan_jobs j ON j.tenant_id=o.tenant_id AND j.simulation_id=o.simulation_id WHERE j.assessment_id=$1`, recovered).Scan(&n); err != nil || n != 3 {
		t.Fatalf("findings: %d %v", n, err)
	}
	var unchanged bool
	if err = env.Owner.QueryRow(ctx, `SELECT snapshot = '{"original":true}'::jsonb FROM reports WHERE id=$1`, report).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("original report changed", err)
	}
	svc := scans.Service{DB: env.App, Engine: &simulation.Engine{}, Reports: &reporting.Service{DB: env.App}}
	if err = svc.Tick(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM reports WHERE assessment_id=$1`, recovered).Scan(&n); err != nil || n != 1 {
		t.Fatalf("report not queued: %d %v", n, err)
	}
	var raw []byte
	if err = env.Owner.QueryRow(ctx, `SELECT snapshot FROM reports WHERE assessment_id=$1`, recovered).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snap reporting.Snapshot
	if err = json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Assets) != 1 || len(snap.Assets[0].Findings) != 3 || snap.Status != "incomplete" {
		t.Fatalf("incorrect recovered snapshot: %+v", snap)
	}
}
