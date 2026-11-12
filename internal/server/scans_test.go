package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
	"orchis-go/internal/server"
	"strings"
	"testing"
)

func TestFirstClassScanParallelIdempotencyPartialFailureAndFrozenReport(t *testing.T) {
	t.Skip("legacy: menguji eksekusi nmap/nuclei per job lama; pipeline orchis-redteam-agent dicakup flow_test.go")
	f := newScanFixture(t, true)
	ctx := context.Background()
	o := createOrg(t, f.a, f.admin, "scan")
	if r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatalf("assign: %d %s", r.Status, r.Body)
	}
	second := mkAsset(t, f.a, f.admin, map[string]any{"category": "host", "name": "target-2", "organization_id": o.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.2"}, {"kind": "url", "value": "http://127.0.0.2:8080/"}}})
	svc := server.ScanService(server.Options{DB: f.a.env.App, Guard: contract.Guard{AllowLoopback: true}, ScanConcurrency: 2})
	in := scans.Input{Language: "id", OrganizationID: o.ID, AssetIDs: []uuid.UUID{uuid.MustParse(f.asset), uuid.MustParse(second)}, Tools: []string{"nmap", "nuclei", "hackbot"}, Preset: "quick"}
	r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "first-class")
	if r.Status != 201 {
		t.Fatalf("scan launch: %d %s", r.Status, r.Body)
	}
	var scan scans.Scan
	r.JSON(&scan)
	if r = f.a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": "en"}, f.analyst); r.Status != 200 {
		t.Fatal("preference change", r.Status)
	}

	again := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "first-class")
	var repeated scans.Scan
	again.JSON(&repeated)
	if again.Status != 201 || repeated.ID != scan.ID {
		t.Fatalf("scan idempotency: %d %s", again.Status, again.Body)
	}
	in.AssetIDs = in.AssetIDs[:1]
	if r = f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "first-class"); r.Status != 409 {
		t.Fatalf("changed options: %d", r.Status)
	}
	var jobs int
	if err := f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM scan_jobs WHERE assessment_id=$1`, scan.ID).Scan(&jobs); err != nil || jobs != 6 {
		t.Fatalf("pairs: %d %v", jobs, err)
	}
	token, err := execution.IssueToken(ctx, f.a.env.App, f.a.env.TenantA, "nuclei")
	if err != nil {
		t.Fatal(err)
	}
	count := func(query string) int {
		t.Helper()
		var n int
		if e := f.a.env.Owner.QueryRow(ctx, query, scan.ID).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	for round := 0; round < 2; round++ {
		if err = svc.Tick(ctx, f.a.env.TenantA); err != nil {
			t.Fatal(err)
		}
		if n := count(`SELECT count(*) FROM scan_jobs WHERE assessment_id=$1 AND status='running'`); n != 2 {
			t.Fatalf("parallel reservations must be two: %d", n)
		}
		if err = svc.Tick(ctx, f.a.env.TenantA); err != nil {
			t.Fatal(err)
		}
		if n := count(`SELECT count(*) FROM executions WHERE simulation_id IN(SELECT simulation_id FROM scan_jobs WHERE assessment_id=$1)`); n != (round+1)*2 {
			t.Fatalf("worker repeated dispatch: %d", n)
		}
		rows, e := f.a.env.Owner.Query(ctx, `SELECT e.id,j.tool FROM executions e JOIN scan_jobs j ON j.tenant_id=e.tenant_id AND j.simulation_id=e.simulation_id WHERE j.assessment_id=$1 AND e.state='ready' ORDER BY e.id`, scan.ID)
		if e != nil {
			t.Fatal(e)
		}
		type task struct{ id, tool string }
		tasks := []task{}
		for rows.Next() {
			var x task
			rows.Scan(&x.id, &x.tool)
			tasks = append(tasks, x)
		}
		rows.Close()
		for i, x := range tasks {
			tok := f.token
			if x.tool == "nuclei" {
				tok = token
			}
			claim := f.inj("POST", "executions/"+x.id+"/claim", nil, tok)
			if claim.Status != 200 {
				t.Fatalf("claim: %d %s", claim.Status, claim.Body)
			}
			var spec execution.ClaimSpec
			claim.JSON(&spec)
			if spec.ScanScope == nil {
				t.Fatal("scan claim omitted injector scope proof")
			}
			body := map[string]any{"exit_code": 0, "findings": []map[string]any{}}
			if round == 0 && i == 0 {
				body = map[string]any{"exit_code": 1, "error": "Web endpoint unreachable from the Orchis server"}
			}
			if x.tool == "nuclei" && !(round == 0 && i == 0) {
				body["findings"] = []map[string]any{{"type": "vulnerability", "value": "fixture@" + x.id, "title": "Exposure password=hunter2", "severity": "high", "details": map[string]any{"template_id": "fixture", "response": "DO-NOT-EXPORT"}}}
			}
			if r = f.inj("POST", "executions/"+x.id+"/result", body, tok); r.Status != 204 {
				t.Fatalf("result: %d %s", r.Status, r.Body)
			}
		}
	}
	if err = svc.Tick(ctx, f.a.env.TenantA); err != nil {
		t.Fatal(err)
	}
	final := f.a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, f.analyst)
	final.JSON(&scan)
	if final.Status != 200 || scan.Status != "finished" {
		t.Fatalf("final: %d %s", final.Status, final.Body)
	}
	failed := 0
	for _, j := range scan.Jobs {
		if j.Status == "failed" {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("partial failure: %+v", scan.Jobs)
	}
	reports := f.a.do("GET", "/api/v1/reports?assessment_id="+scan.ID.String(), nil, f.viewer)
	var page struct{ Items []reporting.Report }
	reports.JSON(&page)
	if reports.Status != 200 || len(page.Items) != 1 || page.Items[0].Snapshot.ScanID == nil {
		t.Fatalf("scan snapshot: %d %s", reports.Status, reports.Body)
	}
	report := page.Items[0]
	if report.Snapshot.Language != "id" {
		t.Fatal("report followed a later preference instead of launch choice", report.Snapshot.Language)
	}

	before := f.a.do("GET", "/api/v1/reports/"+report.ID.String()+"/export/json", nil, f.viewer)
	var frozen reporting.Snapshot
	if err = json.Unmarshal(before.Body, &frozen); err != nil {
		t.Fatal(err)
	}
	if frozen.ComparisonDetails == nil || scan.ComparisonDetails == nil {
		t.Fatal("scan API and JSON must expose frozen comparison")
	}
	for _, asset := range frozen.Assets {
		for _, finding := range asset.Findings {
			if len(finding.Coverage) == 0 {
				t.Fatal("scan freeze lost producing check")
			}
		}
	}
	for _, secret := range []string{"hunter2", "DO-NOT-EXPORT"} {
		if strings.Contains(string(before.Body), secret) {
			t.Fatal("scan report leaked evidence")
		}
	}
	if _, err = f.a.env.Owner.Exec(ctx, `UPDATE findings SET title='changed after freeze',severity='critical' WHERE tenant_id=$1`, f.a.env.TenantA); err != nil {
		t.Fatal(err)
	}
	if err = svc.Tick(ctx, f.a.env.TenantA); err != nil {
		t.Fatal(err)
	}
	after := f.a.do("GET", "/api/v1/reports/"+report.ID.String()+"/export/json", nil, f.viewer)
	if string(before.Body) != string(after.Body) {
		t.Fatal("scan snapshot changed after terminal freeze")
	}
	if count(`SELECT count(*) FROM reports WHERE assessment_id=$1`) != 1 {
		t.Fatal("duplicate scan report")
	}
	other := newApp(t, f.a.env, f.a.env.TenantB)
	other.user(f.a.env.TenantB, "scan@b.test", "admin")
	if r = other.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, other.login("scan@b.test")); r.Status != 404 {
		t.Fatalf("cross tenant scan: %d", r.Status)
	}
}

func TestScanSelectedAssetsRequireAuthorizationOrganisationAndSafeOptions(t *testing.T) {
	f := newScanFixture(t, false)
	o := createOrg(t, f.a, f.admin, "scan-guards")
	in := map[string]any{"organization_id": o.ID, "asset_ids": []string{f.asset}, "tools": []string{"nmap", "nuclei", "hackbot"}, "preset": "quick"}
	if r := f.a.do("POST", "/api/v1/scans", in, f.viewer, "Idempotency-Key", "viewer"); r.Status != 403 {
		t.Fatalf("viewer scan: %d", r.Status)
	}
	if r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "foreign"); r.Status != 422 {
		t.Fatalf("unassigned target: %d", r.Status)
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	if r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "unauthorized"); r.Status != 422 {
		t.Fatalf("unauthorized asset scanned: %d", r.Status)
	}
	in["tools"] = []string{"nmap --script=unsafe"}
	if r := f.a.do("POST", "/api/v1/scans", in, f.admin, "Idempotency-Key", "injection"); r.Status != 422 {
		t.Fatalf("tool injection: %d", r.Status)
	}
	in["tools"] = []string{"nmap", "nuclei", "hackbot"}
	in["params"] = map[string]any{"flags": "--script=unsafe"}
	if r := f.a.do("POST", "/api/v1/scans", in, f.admin, "Idempotency-Key", "free-params"); r.Status != 400 {
		t.Fatalf("free CLI params: %d", r.Status)
	}
}

func TestScanAssetRevokedBeforeDispatchAndCancellationAreIdempotent(t *testing.T) {
	t.Skip("legacy: menguji eksekusi nmap/nuclei per job lama; pipeline orchis-redteam-agent dicakup flow_test.go")
	f := newScanFixture(t, true)
	o := createOrg(t, f.a, f.admin, "scan-revoke")
	ctx := context.Background()
	if r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	in := scans.Input{OrganizationID: o.ID, AssetIDs: []uuid.UUID{uuid.MustParse(f.asset)}, Tools: []string{"nmap", "nuclei", "hackbot"}, Preset: "quick"}
	r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "revoke")
	var scan scans.Scan
	r.JSON(&scan)
	if r.Status != 201 {
		t.Fatalf("launch %d %s", r.Status, r.Body)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE assets SET testing_authorized=false WHERE id=$1`, f.asset); e != nil {
		t.Fatal(e)
	}
	svc := server.ScanService(server.Options{DB: f.a.env.App, Guard: contract.Guard{AllowLoopback: true}})
	if e := svc.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	final := f.a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, f.analyst)
	final.JSON(&scan)
	failedAuth := 0
	for _, j := range scan.Jobs {
		if j.Status == "failed" && strings.Contains(j.Error, "authorization") {
			failedAuth++
		}
	}
	if len(scan.Jobs) == 0 || failedAuth == 0 {
		t.Fatalf("revoked before dispatch: %s", final.Body)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE assets SET testing_authorized=true WHERE id=$1`, f.asset); e != nil {
		t.Fatal(e)
	}
	r = f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "cancel")
	r.JSON(&scan)
	if r.Status != 201 {
		t.Fatalf("launch %d %s", r.Status, r.Body)
	}
	for i := 0; i < 2; i++ {
		if r = f.a.do("POST", "/api/v1/scans/"+scan.ID.String()+"/cancel", nil, f.analyst); r.Status != 200 {
			t.Fatalf("cancel repeated: %d %s", r.Status, r.Body)
		}
	}
	if e := svc.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	var n int
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM executions e JOIN scan_jobs j ON j.simulation_id=e.simulation_id WHERE j.assessment_id=$1`, scan.ID).Scan(&n)
	if n != 0 {
		t.Fatal("canceled scan dispatched jobs")
	}
}

func TestNetworkRangeApprovalScopeRevocationAndDiscoveredAssetIsNotAuthorized(t *testing.T) {
	t.Skip("legacy: pemindaian network range sudah dihapus dari produk")
	f, org, _, enrolled := networkSourceFixture(t)
	o := struct{ ID uuid.UUID }{org}
	ctx := context.Background()
	var r resp
	path := fmt.Sprintf("/api/v1/organizations/%s/agents/%s/networks", o.ID, enrolled.AgentID)
	proposal := f.a.do("GET", path, nil, f.admin)
	if proposal.Status != 200 || !strings.Contains(string(proposal.Body), "127.0.0.1/32") {
		t.Fatalf("proposals %d %s", proposal.Status, proposal.Body)
	}
	approval := scans.ApproveInput{CIDR: "127.0.0.1/32", Authorized: true}
	if r = f.a.do("PUT", path, approval, f.analyst); r.Status != 403 {
		t.Fatalf("non-admin approval %d", r.Status)
	}
	for _, cidr := range []string{"127.0.0.0/24", "0.0.0.0/0", "8.8.8.0/24", "127.0.0.0/23"} {
		bad := approval
		bad.CIDR = cidr
		if r = f.a.do("PUT", path, bad, f.admin); r.Status != 422 {
			t.Fatalf("unreported/oversize network %s %d", cidr, r.Status)
		}
	}
	r = f.a.do("PUT", path, approval, f.admin)
	var approved scans.Range
	r.JSON(&approved)
	if r.Status != 200 || approved.Version != 1 {
		t.Fatalf("approve %d %s", r.Status, r.Body)
	}
	if r = f.a.do("PUT", path, approval, f.admin); r.Status != 409 {
		t.Fatalf("stale approval %d", r.Status)
	}
	svc := server.ScanService(server.Options{DB: f.a.env.App, Guard: contract.Guard{AllowLoopback: true}})
	in := scans.Input{OrganizationID: o.ID, RangeIDs: []uuid.UUID{approved.ID}, Tools: []string{"nmap"}, Preset: "quick"}
	launch := func(key string) scans.Scan {
		t.Helper()
		res := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", key)
		var s scans.Scan
		res.JSON(&s)
		if res.Status != 201 {
			t.Fatalf("network launch %d %s", res.Status, res.Body)
		}
		if e := svc.Tick(ctx, f.a.env.TenantA); e != nil {
			t.Fatal(e)
		}
		return s
	}
	scan := launch("discover")
	var eid string
	f.a.env.Owner.QueryRow(ctx, `SELECT e.id FROM executions e JOIN scan_jobs j ON j.simulation_id=e.simulation_id WHERE j.assessment_id=$1`, scan.ID).Scan(&eid)
	if r = f.inj("POST", "executions/"+eid+"/claim", nil, f.token); r.Status != 200 {
		t.Fatalf("network claim %d %s", r.Status, r.Body)
	}
	if r = f.inj("POST", "executions/"+eid+"/result", okResult, f.token); r.Status != 204 {
		t.Fatalf("network result %d %s", r.Status, r.Body)
	}
	if e := svc.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	result := f.a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, f.analyst)
	result.JSON(&scan)
	if len(scan.Jobs) != 1 || scan.Jobs[0].Status != "finished" {
		t.Fatalf("discovered result %s", result.Body)
	}
	findings := f.a.do("GET", "/api/v1/findings?org="+o.ID.String(), nil, f.viewer)
	var page execution.FindingPage
	findings.JSON(&page)
	if findings.Status != 200 || len(page.Items) != 1 || page.Items[0].AssetID != nil {
		t.Fatalf("unassigned network finding scope %d %s", findings.Status, findings.Body)
	}
	add := f.a.do("POST", "/api/v1/scan-jobs/"+scan.Jobs[0].ID.String()+"/asset", nil, f.analyst)
	var created struct{ ID string }
	add.JSON(&created)
	if add.Status != 201 {
		t.Fatalf("add discovered %d %s", add.Status, add.Body)
	}
	assetResponse := f.a.do("GET", "/api/v1/assets/"+created.ID, nil, f.analyst)
	if !strings.Contains(string(assetResponse.Body), `"testing_authorized":false`) {
		t.Fatalf("discovery granted testing permission: %s", assetResponse.Body)
	}
	scan = launch("revoke-at-claim")
	f.a.env.Owner.QueryRow(ctx, `SELECT e.id FROM executions e JOIN scan_jobs j ON j.simulation_id=e.simulation_id WHERE j.assessment_id=$1`, scan.ID).Scan(&eid)
	approval.Authorized = false
	approval.Version = approved.Version
	r = f.a.do("PUT", path, approval, f.admin)
	if r.Status != 200 {
		t.Fatalf("revoke range %d %s", r.Status, r.Body)
	}
	if r = f.inj("POST", "executions/"+eid+"/claim", nil, f.token); r.Status != 409 {
		t.Fatalf("revoked range reached injector: %d %s", r.Status, r.Body)
	}
	if e := svc.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
}

// Baris dataset ini fixture integrasi sintetis; bukan bukti osquery/scan jaringan nyata.
func networkSourceFixture(t *testing.T) (*agentFixture, uuid.UUID, string, enrollOut) {
	f := newAgentFixture(t)
	o := createOrg(t, f.a, f.admin, "network-source")
	ctx := context.Background()
	token := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "user", "organization_id": o.ID}, f.admin)
	var tk struct{ Token string }
	token.JSON(&tk)
	body := f.enrollBody(tk.Token, "network-fixture")
	body["name"] = "network-fixture"
	r := f.agentCall("POST", "enroll", body, "")
	var enrolled enrollOut
	r.JSON(&enrolled)
	if r.Status != 200 {
		t.Fatalf("enroll %d %s", r.Status, r.Body)
	}
	var asset string
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT asset_id FROM agents WHERE id=$1`, enrolled.AgentID).Scan(&asset); e != nil {
		t.Fatal(e)
	}
	r = f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": asset, "version": 1}}, "testing_authorized": true}, f.admin)
	if r.Status != 204 {
		t.Fatalf("authorization %d %s", r.Status, r.Body)
	}
	// Admin also approves the exact host address. This keeps the independent hostname guard
	// from masking a missing range-revocation check in the Claim mutation.
	r = f.a.do("PATCH", "/api/v1/assets/"+asset, map[string]any{"version": 2, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.1"}}}, f.admin)
	if r.Status != 200 {
		t.Fatalf("admin target confirmation: %d %s", r.Status, r.Body)
	}
	r = f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/osquery/osquery.network_ranges", f.collectionChoice(t, o.ID.String(), "osquery.network_ranges", true), f.admin)
	if r.Status != 204 {
		t.Fatalf("network consent: %d %s", r.Status, r.Body)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `INSERT INTO endpoint_datasets(tenant_id,asset_id,query_key,rows,last_status) VALUES($1,$2,'osquery.network_ranges','[{"interface":"lo","address":"127.0.0.1","mask":"32","source":"interface"}]','ok')`, f.a.env.TenantA, asset); e != nil {
		t.Fatal(e)
	}

	return f, o.ID, asset, enrolled
}
func TestNetworkSourceRequiresCurrentConsentBindingAndFreshSuccessfulData(t *testing.T) {
	f, org, asset, enrolled := networkSourceFixture(t)
	ctx := context.Background()
	path := fmt.Sprintf("/api/v1/organizations/%s/agents/%s/networks", org, enrolled.AgentID)
	tests := []struct {
		name, change, restore string
		arg                   any
	}{
		{"consent", `UPDATE osquery_org_settings SET consent_version='old' WHERE organization_id=$1`, `UPDATE osquery_org_settings SET consent_version='` + osquery.ConsentVersion + `' WHERE organization_id=$1`, org},
		{"disabled", `UPDATE osquery_org_settings SET enabled=false WHERE organization_id=$1`, `UPDATE osquery_org_settings SET enabled=true WHERE organization_id=$1`, org},
		{"freshness", `UPDATE endpoint_datasets SET last_attempt_at=now()-interval '25 hours' WHERE asset_id=$1`, `UPDATE endpoint_datasets SET last_attempt_at=now() WHERE asset_id=$1`, asset},
		{"last success", `UPDATE endpoint_datasets SET last_status='failed' WHERE asset_id=$1`, `UPDATE endpoint_datasets SET last_status='ok' WHERE asset_id=$1`, asset},
		{"revoked agent", `UPDATE agents SET status='revoked' WHERE id=$1`, `UPDATE agents SET status='online' WHERE id=$1`, enrolled.AgentID},
		{"demo agent", `UPDATE agents SET is_demo=true WHERE id=$1`, `UPDATE agents SET is_demo=false WHERE id=$1`, enrolled.AgentID},
		{"testing permission", `UPDATE assets SET testing_authorized=false WHERE id=$1`, `UPDATE assets SET testing_authorized=true WHERE id=$1`, asset},
		{"asset organisation", `UPDATE assets SET organization_id=NULL WHERE id=$1`, `UPDATE assets SET organization_id='` + org.String() + `' WHERE id=$1`, asset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, e := f.a.env.Owner.Exec(ctx, tt.change, tt.arg); e != nil {
				t.Fatal(e)
			}
			r := f.a.do("GET", path, nil, f.admin)
			var out scans.Networks
			r.JSON(&out)
			if _, e := f.a.env.Owner.Exec(ctx, tt.restore, tt.arg); e != nil {
				t.Fatal(e)
			}
			if r.Status != 200 || out.SourceError == "" || len(out.Reported) != 0 {
				t.Fatalf("unsafe network source accepted (%s): %d %s", tt.name, r.Status, r.Body)
			}
		})
	}
}
func TestNetworkScanDiscardsResultsAfterRangeApprovalChanges(t *testing.T) {
	t.Skip("legacy: pemindaian network range sudah dihapus dari produk")
	f, org, _, enrolled := networkSourceFixture(t)
	ctx := context.Background()
	path := fmt.Sprintf("/api/v1/organizations/%s/agents/%s/networks", org, enrolled.AgentID)
	r := f.a.do("PUT", path, scans.ApproveInput{CIDR: "127.0.0.1/32", Authorized: true}, f.admin)
	var approved scans.Range
	r.JSON(&approved)
	if r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	in := scans.Input{OrganizationID: org, RangeIDs: []uuid.UUID{approved.ID}, Tools: []string{"nmap"}, Preset: "quick"}
	r = f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "result-revoke")
	var scan scans.Scan
	r.JSON(&scan)
	if r.Status != 201 {
		t.Fatal(r.Status, string(r.Body))
	}
	svc := server.ScanService(server.Options{DB: f.a.env.App, Guard: contract.Guard{AllowLoopback: true}})
	if e := svc.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	var eid string
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT e.id FROM executions e JOIN scan_jobs j ON j.simulation_id=e.simulation_id WHERE j.assessment_id=$1`, scan.ID).Scan(&eid); e != nil {
		t.Fatal(e)
	}
	if r = f.inj("POST", "executions/"+eid+"/claim", nil, f.token); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = f.a.do("PUT", path, scans.ApproveInput{CIDR: "127.0.0.1/32", Authorized: false, Version: approved.Version}, f.admin); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = f.inj("POST", "executions/"+eid+"/result", okResult, f.token); r.Status != 204 {
		t.Fatal(r.Status, string(r.Body))
	}
	var n int
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM finding_occurrences WHERE execution_id=$1`, eid).Scan(&n)
	if n != 0 {
		t.Fatal("revoked scan result was ingested")
	}
}

func TestNetworkApprovalNeedsSettingsManageAndAssetAuthorize(t *testing.T) {
	f, org, _, enrolled := networkSourceFixture(t)
	ctx := context.Background()
	path := fmt.Sprintf("/api/v1/organizations/%s/agents/%s/networks", org, enrolled.AgentID)
	body := scans.ApproveInput{CIDR: "127.0.0.1/32", Authorized: true}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE roles SET permissions=array_append(permissions,'asset.authorize') WHERE tenant_id=$1 AND key='analyst'`, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if r := f.a.do("PUT", path, body, f.analyst); r.Status != 403 {
		t.Fatalf("range approval without settings.manage: %d", r.Status)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE roles SET permissions=array_append(array_remove(permissions,'asset.authorize'),'settings.manage') WHERE tenant_id=$1 AND key='analyst'`, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if r := f.a.do("PUT", path, body, f.analyst); r.Status != 403 {
		t.Fatalf("range approval without asset.authorize: %d", r.Status)
	}
}

func TestScanPreservesEverySelectedAssetEvenWhenAddressesMatch(t *testing.T) {
	f := newScanFixture(t, true)
	org := createOrg(t, f.a, f.admin, "matching-address")
	if r := f.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	other := mkAsset(t, f.a, f.admin, map[string]any{"name": "Same host, distinct asset", "category": "host", "organization_id": org.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.1"}, {"kind": "url", "value": "http://127.0.0.1:8081/"}}})
	body := scans.Input{OrganizationID: org.ID, AssetIDs: []uuid.UUID{uuid.MustParse(f.asset), uuid.MustParse(other)}, Tools: []string{"nmap", "nuclei", "hackbot"}, Preset: "quick"}
	r := f.a.do("POST", "/api/v1/scans", body, f.analyst, "Idempotency-Key", "same-host")
	var scan scans.Scan
	r.JSON(&scan)
	if r.Status != 201 || len(scan.Jobs) != 6 {
		t.Fatalf("selected asset/tool pair disappeared on shared address: %d %s", r.Status, r.Body)
	}
	counts := map[string]int{}
	for _, j := range scan.Jobs {
		if j.AssetID == nil {
			t.Fatal("selected asset lost its identity")
		}
		counts[j.AssetID.String()]++
	}
	if counts[f.asset] != 3 || counts[other] != 3 {
		t.Fatal("shared-address findings would lose asset scope")
	}
}
