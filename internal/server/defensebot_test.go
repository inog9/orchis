package server_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/contract"
	"orchis-go/internal/defensebot"
	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
	"orchis-go/internal/server"
)

// defenseKeys are the orchis-defense-agent checks an administrator approves for a Linux endpoint.
func defenseKeys() []string {
	var keys []string
	for _, key := range defensebot.Queries {
		if q := osquery.Find(key); q != nil && q.SupportsOS("linux") {
			keys = append(keys, key)
		}
	}
	return keys
}

func defenseRows(key string) []map[string]any {
	switch key {
	case defensebot.QueryOSRelease:
		return []map[string]any{{"name": "Ubuntu", "version": "24.04.1 LTS (Noble Numbat)", "major": "24", "minor": "4", "platform": "ubuntu", "platform_like": "debian", "codename": "noble"}}
	case defensebot.QueryPackages:
		return []map[string]any{
			{"name": "libssl3t64", "source_name": "openssl", "version": "3.0.13-0ubuntu3.4", "arch": "amd64", "pkg_type": "deb", "epoch": "", "release": ""},
			{"name": "nginx", "source_name": "nginx", "version": "1.24.0-2ubuntu7", "arch": "amd64", "pkg_type": "deb", "epoch": "", "release": ""},
		}
	case defensebot.QueryKernel:
		return []map[string]any{{"version": "6.8.0-45-generic"}}
	case defensebot.QueryExposure:
		return []map[string]any{{"port": "443", "protocol": "6", "address": "0.0.0.0", "process": "nginx", "exe_path": "/usr/sbin/nginx", "owner_package": "nginx"}}
	case defensebot.QueryHardening:
		return []map[string]any{{"item": "sysctl", "target": "kernel.randomize_va_space", "value": "2"}}
	case defensebot.QueryFirewall:
		return []map[string]any{{"iptables_rules": "4", "active_firewall_units": "1"}}
	}
	return []map[string]any{}
}

// answerCollections plays the agent: it claims every queued osquery collection and returns realistic datasets.
func answerCollections(t *testing.T, f *agentFixture, secret string, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		cl := f.agentCall("POST", "executions/claim", nil, secret)
		if cl.Status != 200 {
			t.Fatalf("agent claim %d/%d: %d %s", i+1, want, cl.Status, cl.Body)
		}
		var spec execution.ClaimSpec
		cl.JSON(&spec)
		code := 0
		body := map[string]any{"exit_code": code, "datasets": []map[string]any{{"query": spec.ContractKey, "status": "ok", "rows": defenseRows(spec.ContractKey)}}}
		if r := f.agentCall("POST", "executions/"+spec.ExecutionID.String()+"/result", body, secret); r.Status != 204 {
			t.Fatalf("agent result %s: %d %s", spec.ContractKey, r.Status, r.Body)
		}
	}
}

func TestOrchisDefenseAgentScanLifecycle(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	env := f.a.env
	keys := defenseKeys()
	secret, asset := f.approvedAgent(t, "defense-box", keys...)
	var org uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT organization_id FROM assets WHERE id=$1`, asset).Scan(&org); err != nil {
		t.Fatal(err)
	}
	assetID := uuid.MustParse(asset)
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)
	in := scans.Input{OrganizationID: org, AssetIDs: []uuid.UUID{assetID}, Tools: []string{"defensebot"}}

	// Validation: a viewer cannot launch; tools cannot be mixed; legacy hackbot pipeline still needs a web target.
	if r := f.a.do("POST", "/api/v1/scans", in, f.viewer, "Idempotency-Key", "d-viewer"); r.Status != 403 {
		t.Fatalf("viewer launch: %d %s", r.Status, r.Body)
	}
	mixed := in
	mixed.Tools = []string{"defensebot", "nmap"}
	if r := f.a.do("POST", "/api/v1/scans", mixed, f.analyst, "Idempotency-Key", "d-mixed"); r.Status != 422 {
		t.Fatalf("mixed tools must be refused: %d %s", r.Status, r.Body)
	}

	launch := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "d-1")
	if launch.Status != 201 {
		t.Fatalf("launch: %d %s", launch.Status, launch.Body)
	}
	var scan scans.Scan
	launch.JSON(&scan)
	again := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "d-1")
	var dup scans.Scan
	again.JSON(&dup)
	if again.Status != 201 || dup.ID != scan.ID {
		t.Fatal("idempotency failed")
	}
	if len(scan.Jobs) != 1 || scan.Jobs[0].Tool != "defensebot" || scan.Jobs[0].Status != "waiting" {
		t.Fatalf("job: %+v", scan.Jobs)
	}
	var pending int
	env.Owner.QueryRow(ctx, `SELECT count(*) FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id WHERE e.target_asset_id=$1 AND e.executor_kind='agent' AND e.state='ready' AND c.key LIKE 'osquery.%'`, asset).Scan(&pending)
	if pending != len(keys) {
		t.Fatalf("launch must queue one fresh collection per approved check: %d vs %d", pending, len(keys))
	}

	svc := server.ScanService(server.Options{DB: env.App, Guard: contract.Guard{AllowLoopback: true}})
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	var status string
	env.Owner.QueryRow(ctx, `SELECT status FROM scan_jobs WHERE assessment_id=$1`, scan.ID).Scan(&status)
	if status != "waiting" {
		t.Fatalf("orchis-defense-agent must wait for the fresh collection, job is %s", status)
	}

	answerCollections(t, f, secret, len(keys))
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	env.Owner.QueryRow(ctx, `SELECT status FROM scan_jobs WHERE assessment_id=$1`, scan.ID).Scan(&status)
	if status != "running" {
		var why string
		env.Owner.QueryRow(ctx, `SELECT error FROM scan_jobs WHERE assessment_id=$1`, scan.ID).Scan(&why)
		t.Fatalf("job must be dispatched after the collection finished, got %s: %s", status, why)
	}
	var execID uuid.UUID
	var target string
	if err := env.Owner.QueryRow(ctx, `SELECT x.id,x.target_address FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1`, scan.ID).Scan(&execID, &target); err != nil {
		t.Fatal(err)
	}
	if target != "local" {
		t.Fatalf("orchis-defense-agent has no network target: %q", target)
	}

	// Only the orchis-defense-agent worker token can claim it.
	hackToken, err := execution.IssueToken(ctx, env.App, env.TenantA, "hackbot")
	if err != nil {
		t.Fatal(err)
	}
	if r := f.a.do("POST", "/api/v1/injector/executions/"+execID.String()+"/claim", nil, "", "X-Requested-With", "", "Authorization", "Bearer "+hackToken); r.Status != 404 {
		t.Fatalf("another worker must not claim it: %d %s", r.Status, r.Body)
	}
	token, err := execution.IssueToken(ctx, env.App, env.TenantA, "defensebot")
	if err != nil {
		t.Fatal(err)
	}
	inj := func(path string, body any) resp {
		return f.a.do("POST", "/api/v1/injector/executions/"+execID.String()+path, body, "", "X-Requested-With", "", "Authorization", "Bearer "+token)
	}
	claim := inj("/claim", nil)
	if claim.Status != 200 {
		t.Fatalf("claim: %d %s", claim.Status, claim.Body)
	}
	var spec execution.ClaimSpec
	claim.JSON(&spec)
	if spec.Defense == nil {
		t.Fatal("claim must carry the stored datasets")
	}
	pk := spec.Defense.Datasets[defensebot.QueryPackages]
	if pk.Status != "ok" || len(pk.Rows) != 2 || pk.CollectedAt == nil || spec.Defense.Language != "en" {
		t.Fatalf("package dataset: %+v language=%q", pk, spec.Defense.Language)
	}
	if spec.Defense.Datasets[defensebot.QueryApps].Status != "never" {
		t.Fatalf("a check that never ran must say so: %+v", spec.Defense.Datasets[defensebot.QueryApps])
	}
	if got := inj("/claim", nil); got.Status != 409 {
		t.Fatal("duplicate claim accepted")
	}

	// Result: a vulnerability, a hardening finding, and a limited summary.
	code := 0
	details, _ := json.Marshal(map[string]any{"cve": "CVE-2024-0001", "package": "libssl3t64", "installed_version": "3.0.13-0ubuntu3.4", "fixed_version": "3.0.13-0ubuntu3.5", "kev": true, "exposure": "exposed", "method": "osv-distro", "secret_field": "must-not-reach-reports"})
	sum := defensebot.Summary{Schema: 1, Disposition: "limited", Platform: "ubuntu", SoftwareChecked: 2, BySeverity: map[string]int{"high": 1},
		Findings: 1, KnownExploited: 1, Notes: []defensebot.Note{{Code: "apps_unmapped", Count: 3, Limiting: true}},
		Provenance: defensebot.Provenance{Engine: defensebot.EngineVersion, Ecosystem: "Ubuntu:24.04:LTS", QueriedAt: time.Now().UTC(), OSVPairs: 2, SeverityPolicy: defensebot.SeverityPolicy, Sources: []string{"OSV (osv.dev)"}}}
	result := execution.Result{ExitCode: &code, Defense: &sum, Findings: []execution.NewFinding{
		{Type: "vulnerability", Value: "defensebot:ubuntu:24.04:lts:openssl:cve-2024-0001", Title: "CVE-2024-0001 in libssl3t64 3.0.13-0ubuntu3.4", Severity: "high", Details: details},
		{Type: "text", Value: "osquery:no-host-firewall", Title: "No active host firewall was detected", Severity: "low", Details: json.RawMessage(`{"reason":"x"}`)},
		{Type: "open_port", Value: "tcp/1", Title: "forged type", Severity: "critical", Details: json.RawMessage(`{}`)},
	}}
	if bad := inj("/result", execution.Result{ExitCode: &code, Findings: result.Findings[:1]}); bad.Status != 400 {
		t.Fatalf("a orchis-defense-agent result without a summary must be refused: %d %s", bad.Status, bad.Body)
	}
	if got := inj("/result", result); got.Status != 204 {
		t.Fatalf("result: %d %s", got.Status, got.Body)
	}
	if got := inj("/result", result); got.Status != 409 {
		t.Fatal("duplicate result accepted")
	}
	var types string
	env.Owner.QueryRow(ctx, `SELECT string_agg(f.type||':'||f.severity, ',' ORDER BY f.type) FROM finding_occurrences o JOIN findings f ON f.id=o.finding_id WHERE o.execution_id=$1`, execID).Scan(&types)
	if types != "text:low,vulnerability:high" {
		t.Fatalf("forged finding types must be rejected: %q", types)
	}
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	env.Owner.QueryRow(ctx, `SELECT status FROM scan_jobs WHERE assessment_id=$1`, scan.ID).Scan(&status)
	if status != "incomplete" {
		t.Fatalf("a limited run must make the job incomplete, not finished: %s", status)
	}
	var assessment string
	env.Owner.QueryRow(ctx, `SELECT status FROM assessments WHERE id=$1`, scan.ID).Scan(&assessment)
	if assessment != "incomplete" {
		t.Fatalf("assessment: %s", assessment)
	}

	var snapRaw []byte
	if err := env.Owner.QueryRow(ctx, `SELECT snapshot FROM reports WHERE assessment_id=$1`, scan.ID).Scan(&snapRaw); err != nil {
		t.Fatalf("report was not frozen: %v", err)
	}
	var snap reporting.Snapshot
	if err := json.Unmarshal(snapRaw, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Assets) != 1 || snap.Assets[0].Defense == nil || snap.Assets[0].Defense.Disposition != "limited" || snap.BySeverity["high"] != 1 {
		t.Fatalf("snapshot: %+v", snap.Assets)
	}
	a0 := snap.Assets[0]
	if len(a0.Methods) != 1 || a0.Methods[0].Action != contract.KeyDefenseBot || a0.Methods[0].Outcome != "incomplete" || !strings.Contains(string(a0.Methods[0].Params), "defensebot/1") {
		t.Fatalf("method provenance: %+v", a0.Methods)
	}
	foundVuln := false
	for _, fd := range a0.Findings {
		if fd.Type == "vulnerability" {
			foundVuln = true
			if fd.Evidence["cve"] != "CVE-2024-0001" || fd.Evidence["fixed_version"] != "3.0.13-0ubuntu3.5" || fd.Evidence["kev"] != true || fd.Evidence["exposure"] != "exposed" {
				t.Fatalf("evidence: %+v", fd.Evidence)
			}
			if _, leaked := fd.Evidence["secret_field"]; leaked {
				t.Fatal("non-whitelisted evidence leaked into the report")
			}
		}
	}
	if !foundVuln {
		t.Fatalf("vulnerability missing from the report: %+v", a0.Findings)
	}
	got := f.a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, f.analyst)
	var detail scans.Scan
	got.JSON(&detail)
	if got.Status != 200 || len(detail.Jobs) != 1 || detail.Jobs[0].Defense == nil || detail.Jobs[0].Defense.KnownExploited != 1 {
		t.Fatalf("scan detail must expose the orchis-defense-agent summary: %d %s", got.Status, got.Body)
	}
}

func TestOrchisDefenseAgentLaunchRefusesUnreadyEndpoints(t *testing.T) {
	f := newAgentFixture(t)
	// Authorized agent, but no collection approved for orchis-defense-agent.
	secret, asset := f.approvedAgent(t, "unapproved-box")
	ctx := context.Background()
	var org uuid.UUID
	if err := f.a.env.Owner.QueryRow(ctx, `SELECT organization_id FROM assets WHERE id=$1`, asset).Scan(&org); err != nil {
		t.Fatal(err)
	}
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)
	in := scans.Input{OrganizationID: org, AssetIDs: []uuid.UUID{uuid.MustParse(asset)}, Tools: []string{"defensebot"}}
	if r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "d-unapproved"); r.Status != 409 || !strings.Contains(string(r.Body), "not approved") {
		t.Fatalf("unapproved collection must refuse the launch: %d %s", r.Status, r.Body)
	}
	var n int
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM assessments WHERE organization_id=$1`, org).Scan(&n)
	if n != 0 {
		t.Fatalf("a refused launch must not leave an assessment behind: %d", n)
	}
	// An asset without testing authorization is refused.
	in.AssetIDs = []uuid.UUID{uuid.New()}
	if r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "d-missing"); r.Status != 422 {
		t.Fatalf("unknown asset: %d %s", r.Status, r.Body)
	}
}
