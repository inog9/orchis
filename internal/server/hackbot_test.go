package server_test

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http/httptest"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
	"orchis-go/internal/server"
	"strings"
	"testing"
)

func TestHackbotScanLifecycle(t *testing.T) {
	for _, scenario := range []string{"complete", "limited", "input_required", "scanner_failed", "reviewed_limited", "reviewed_input_required", "reviewed_scanner_failed"} {
		t.Run(scenario, func(t *testing.T) {
			disposition := scenario
			reviewed := strings.HasPrefix(scenario, "reviewed_")
			if reviewed {
				disposition = strings.TrimPrefix(scenario, "reviewed_")
			}
			failedScanner := ""
			if disposition == "scanner_failed" {
				disposition = "complete"
				if reviewed {
					disposition = "limited"
				}
				failedScanner = "nmap"
			}
			a, env := setup(t)
			ctx := context.Background()
			admin := a.login("admin@a.test")
			analyst := a.login("analyst@a.test")
			viewer := a.login("viewer@a.test")
			org := createOrg(t, a, admin, "pentest")
			asset := mkAsset(t, a, admin, map[string]any{"category": "web_application", "name": "Test web", "organization_id": org.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.1:8080/"}}})
			in := scans.Input{OrganizationID: org.ID, AssetIDs: []uuid.UUID{uuid.MustParse(asset)}}
			launch := a.do("POST", "/api/v1/scans", in, analyst, "Idempotency-Key", "hackbot-fixture")
			if launch.Status != 201 {
				t.Fatalf("launch: %d %s", launch.Status, launch.Body)
			}
			var scan scans.Scan
			launch.JSON(&scan)
			var discoveryTarget string
			var discoveryParams []byte
			if err := env.Owner.QueryRow(ctx, `SELECT target,params FROM scan_jobs WHERE assessment_id=$1 AND tool='nmap'`, scan.ID).Scan(&discoveryTarget, &discoveryParams); err != nil {
				t.Fatal(err)
			}
			if discoveryTarget != "127.0.0.1" || !strings.Contains(string(discoveryParams), `"ports": "8080"`) {
				t.Fatalf("web discovery escaped origin: %s %s", discoveryTarget, discoveryParams)
			}

			again := a.do("POST", "/api/v1/scans", in, analyst, "Idempotency-Key", "hackbot-fixture")
			var duplicate scans.Scan
			again.JSON(&duplicate)
			if again.Status != 201 || duplicate.ID != scan.ID {
				t.Fatal("idempotency failed")
			}
			old := in
			old.Tools = []string{"nmap"}
			if got := a.do("POST", "/api/v1/scans", old, analyst, "Idempotency-Key", "old"); got.Status != 422 {
				t.Fatalf("legacy scan accepted: %d %s", got.Status, got.Body)
			}
			svc := server.ScanService(server.Options{DB: env.App, Guard: contract.Guard{AllowLoopback: true}})
			if err := svc.Tick(ctx, env.TenantA); err != nil {
				t.Fatal(err)
			}
			finishPreliminaryScanners(t, a, scan.ID, failedScanner)
			var id uuid.UUID
			if err := env.Owner.QueryRow(ctx, `SELECT x.id FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1 AND j.tool='hackbot'`, scan.ID).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if _, err := env.Owner.Exec(ctx, `UPDATE findings SET title='Mutated current finding' WHERE tenant_id=$1`, env.TenantA); err != nil {
				t.Fatal(err)
			}
			token, err := execution.IssueToken(ctx, env.App, env.TenantA, "hackbot")
			if err != nil {
				t.Fatal(err)
			}
			inj := func(path string, body any) resp {
				return a.do("POST", "/api/v1/injector/executions/"+id.String()+path, body, "", "X-Requested-With", "", "Authorization", "Bearer "+token)
			}
			claim := inj("/claim", nil)
			if claim.Status != 200 {
				t.Fatalf("claim: %d %s", claim.Status, claim.Body)
			}
			var spec execution.ClaimSpec
			claim.JSON(&spec)
			if len(spec.ScannerInputs) != 2 {
				t.Fatalf("missing scanner handoff: %+v", spec.ScannerInputs)
			}
			for _, input := range spec.ScannerInputs {
				if input.Tool == failedScanner {
					if input.Status != "failed" || input.Error == "" {
						t.Fatal("failed scanner metadata lost")
					}
					continue
				}
				if !strings.Contains(string(input.Findings), "Preliminary "+input.Tool) {
					t.Fatal("scanner findings were not snapshotted into claim")
				}
			}
			if got := inj("/claim", nil); got.Status != 409 {
				t.Fatal("duplicate claim accepted")
			}
			if got := inj("/progress", hackbot.Progress{SessionID: "session-1", Phase: "recon"}); got.Status != 204 {
				t.Fatalf("progress: %d %s", got.Status, got.Body)
			}
			code := 0
			if disposition == "limited" {
				code = 2
			}
			if disposition == "input_required" {
				code = 3
			}
			result := execution.Result{ExitCode: &code, Hackbot: &hackbot.Result{Schema: 1, ReviewCompleted: reviewed, SessionID: "session-1", Disposition: disposition, Phase: "report", Summary: strings.Repeat("tested ", 350), Report: "# Full fixture report\n\ntoken=supersecret", Validation: "# Internal validation ledger", Findings: []hackbot.Finding{{ID: "f1", Title: "Verified issue", Value: "http://127.0.0.1:8080/#f1", Severity: "medium", Details: json.RawMessage(`{"verdict":"VALIDATED","reason":"Controlled fixture evidence"}`)}}}}
			if got := inj("/result", result); got.Status != 204 {
				t.Fatalf("result: %d %s", got.Status, got.Body)
			}
			if got := inj("/result", result); got.Status != 409 {
				t.Fatal("duplicate result accepted")
			}
			if err := svc.Tick(ctx, env.TenantA); err != nil {
				t.Fatal(err)
			}
			got := a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, analyst)
			got.JSON(&scan)
			if got.Status != 200 || len(scan.Jobs) != 3 {
				t.Fatalf("results: %d %s", got.Status, got.Body)
			}
			want := "finished"
			if disposition != "complete" {
				want = "incomplete"
			}
			overall := want
			if reviewed && failedScanner == "" {
				overall = "finished"
			}
			if failedScanner != "" {
				overall = "incomplete"
			}
			if scan.Status != overall || hackbotJob(scan.Jobs).Status != want {
				t.Fatalf("coverage mislabeled: %+v", scan)
			}
			if scan.LimitedCoverage != (disposition != "complete" || failedScanner != "") {
				t.Fatal("coverage flag is incorrect")
			}
			if len(strings.Fields(scan.Summary)) > 300 {
				t.Fatal("summary exceeds limit")
			}
			if strings.Contains(string(got.Body), "Internal validation ledger") {
				t.Fatal("internal ledger exposed")
			}
			ledgerURL := "/api/v1/scan-jobs/" + hackbotJob(scan.Jobs).ID.String() + "/validation"
			if r := a.do("GET", ledgerURL, nil, analyst); r.Status != 403 {
				t.Fatalf("analyst got internal ledger: %d", r.Status)
			}
			if r := a.do("GET", ledgerURL, nil, admin); r.Status != 200 {
				t.Fatalf("admin ledger: %d %s", r.Status, r.Body)
			}

			review := a.do("GET", "/api/v1/scans/"+scan.ID.String()+"/recommendations", nil, analyst)
			if review.Status != 200 || !strings.Contains(string(review.Body), `"status":"queued"`) {
				t.Fatalf("completed scan did not queue its OpenCode review: %d %s", review.Status, review.Body)
			}
			var reviewCount int
			if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM scan_recommendations WHERE scan_id=$1`, scan.ID).Scan(&reviewCount); err != nil || reviewCount != 1 {
				t.Fatalf("review was not queued exactly once: %d %v", reviewCount, err)
			}
			var reportID uuid.UUID
			var snapshot reporting.Snapshot
			var data []byte
			if err := env.Owner.QueryRow(ctx, `SELECT id,snapshot FROM reports WHERE assessment_id=$1`, scan.ID).Scan(&reportID, &data); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &snapshot); err != nil {
				t.Fatal(err)
			}
			findingCount := 3
			if failedScanner != "" {
				findingCount = 2
			}
			if len(snapshot.Assets[0].Methods) != 3 || len(snapshot.Assets[0].Findings) != findingCount {
				t.Fatal("combined report lost preliminary findings or methods")
			}
			if snapshot.Summary == "" || strings.Contains(string(data), "supersecret") {
				t.Fatal("report missing or unredacted")
			}
			if (disposition != "complete" || failedScanner != "") && reporting.Coverage(snapshot.Assets).Tested != 0 {
				t.Fatal("partial coverage counted as tested")
			}
			if _, err := env.Owner.Exec(ctx, `UPDATE reports SET status='ready' WHERE id=$1`, reportID); err != nil {
				t.Fatal(err)
			}
			previewServer := httptest.NewServer(server.New(server.Options{DB: env.App, Tenant: env.TenantA, Guard: contract.Guard{AllowLoopback: true}, Store: hackbotPDFStore{}}))
			defer previewServer.Close()
			previewApp := &app{t: t, env: env, srv: previewServer}
			pdf := previewApp.do("GET", "/api/v1/reports/"+reportID.String()+"/export/pdf?preview=1", nil, viewer)
			if pdf.Status != 200 || pdf.Header.Get("X-Frame-Options") != "SAMEORIGIN" || !strings.HasPrefix(pdf.Header.Get("Content-Disposition"), "inline") {
				t.Fatalf("PDF cannot be previewed in Orchis: %d %+v", pdf.Status, pdf.Header)
			}
			jsonView := previewApp.do("GET", "/api/v1/reports/"+reportID.String(), nil, viewer)
			if jsonView.Header.Get("X-Frame-Options") != "DENY" {
				t.Fatal("non-PDF frame protection changed")
			}
			if r := a.do("GET", "/api/v1/reports/"+reportID.String(), nil, viewer); r.Status != 200 || !strings.Contains(string(r.Body), "Verified issue") {
				t.Fatalf("download: %d %s", r.Status, r.Body)
			}
		})
	}
}

func TestHackbotRevocationAndLeaseExpiry(t *testing.T) {
	for _, mode := range []string{"revoked", "lease"} {
		t.Run(mode, func(t *testing.T) {
			a, env := setup(t)
			ctx := context.Background()
			admin := a.login("admin@a.test")
			analyst := a.login("analyst@a.test")
			org := createOrg(t, a, admin, "lease")
			asset := mkAsset(t, a, admin, map[string]any{"category": "web_application", "name": "Scope fixture", "organization_id": org.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.1:8080/"}}})
			response := a.do("POST", "/api/v1/scans", scans.Input{OrganizationID: org.ID, AssetIDs: []uuid.UUID{uuid.MustParse(asset)}}, analyst, "Idempotency-Key", "scope")
			if response.Status != 201 {
				t.Fatalf("launch %d %s", response.Status, response.Body)
			}
			var scan scans.Scan
			response.JSON(&scan)
			opts := server.Options{DB: env.App, Tenant: env.TenantA, Guard: contract.Guard{AllowLoopback: true}}
			svc := server.ScanService(opts)
			if err := svc.Tick(ctx, env.TenantA); err != nil {
				t.Fatal(err)
			}
			finishPreliminaryScanners(t, a, scan.ID, "")
			var id uuid.UUID
			if err := env.Owner.QueryRow(ctx, `SELECT x.id FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1 AND j.tool='hackbot'`, scan.ID).Scan(&id); err != nil {
				t.Fatal(err)
			}
			token, err := execution.IssueToken(ctx, env.App, env.TenantA, "hackbot")
			if err != nil {
				t.Fatal(err)
			}
			rt := server.Runtime(opts)
			in, err := rt.Authenticate(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = rt.Claim(ctx, in, id); err != nil {
				t.Fatal(err)
			}
			if mode == "revoked" {
				if _, err = env.Owner.Exec(ctx, `UPDATE assets SET testing_authorized=false WHERE id=$1`, asset); err != nil {
					t.Fatal(err)
				}
				cancel, err := rt.Heartbeat(ctx, in, id)
				if err != nil || !cancel {
					t.Fatalf("revocation failed: %v %v", cancel, err)
				}
			} else {
				if _, err = env.Owner.Exec(ctx, `UPDATE executions SET lease_until=now()-interval '1 minute',max_attempts=5 WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				retried, timedOut, err := rt.Sweep(ctx)
				if err != nil || retried != 0 || timedOut != 1 {
					t.Fatalf("ambiguous engagement retried: %d %d %v", retried, timedOut, err)
				}
			}
			if err = svc.Tick(ctx, env.TenantA); err != nil {
				t.Fatal(err)
			}
			response = a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, analyst)
			response.JSON(&scan)
			if scan.Status == "running" || len(scan.Jobs) != 3 || hackbotJob(scan.Jobs).Status == "finished" {
				t.Fatalf("unsafe terminal status: %+v", scan)
			}
			retained, err := rt.RetainWorkspace(ctx, in, id)
			if err != nil || !retained {
				t.Fatal("retained evidence was discarded")
			}
			retained, err = rt.RetainWorkspace(ctx, in, uuid.New())
			if err != nil || retained {
				t.Fatal("deleted evidence cannot be pruned")
			}
		})
	}
}

type hackbotPDFStore struct{}

func (hackbotPDFStore) Put(context.Context, string, []byte, string) error { return nil }
func (hackbotPDFStore) Get(context.Context, string) ([]byte, error) {
	return []byte("%PDF-local-fixture"), nil
}

func hackbotJob(jobs []scans.Job) scans.Job {
	for _, job := range jobs {
		if job.Tool == "hackbot" {
			return job
		}
	}
	return scans.Job{}
}

func finishPreliminaryScanners(t *testing.T, a *app, scan uuid.UUID, failed string) {
	t.Helper()
	ctx := context.Background()
	env := a.env
	opts := server.Options{DB: env.App, Tenant: env.TenantA, Guard: contract.Guard{AllowLoopback: true}}
	svc := server.ScanService(opts)
	rt := server.Runtime(opts)
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	for index, tool := range []string{"nmap", "nuclei"} {
		var status string
		if err := env.Owner.QueryRow(ctx, `SELECT status FROM scan_jobs WHERE assessment_id=$1 AND tool='hackbot'`, scan).Scan(&status); err != nil || status != "waiting" {
			t.Fatalf("orchis-redteam-agent ran before both scanners finished: %s %v", status, err)
		}
		var executionID uuid.UUID
		if err := env.Owner.QueryRow(ctx, `SELECT x.id FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1 AND j.tool=$2`, scan, tool).Scan(&executionID); err != nil {
			t.Fatal(err)
		}
		token, err := execution.IssueToken(ctx, env.App, env.TenantA, tool)
		if err != nil {
			t.Fatal(err)
		}
		actor, err := rt.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rt.Claim(ctx, actor, executionID); err != nil {
			t.Fatal(err)
		}
		code := 0
		typ := "vulnerability"
		severity := "medium"
		if tool == "nmap" {
			typ = "open_port"
			severity = "info"
		}
		result := execution.Result{ExitCode: &code, Findings: []execution.NewFinding{{Type: typ, Title: "Preliminary " + tool, Value: "http://127.0.0.1:8080/#" + tool, Severity: severity, Details: json.RawMessage(`{"source":"fixture"}`)}}}
		if tool == failed {
			code = 1
			result.Error = "Fixture scanner failed"
			result.Findings = nil
		}
		if err = rt.Complete(ctx, actor, executionID, result); err != nil {
			t.Fatal(err)
		}
		if err = svc.Tick(ctx, env.TenantA); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			continue
		}
	}
}

func TestHackbotRevokedPreliminaryCancelsDependent(t *testing.T) {
	a, env := setup(t)
	ctx := context.Background()
	admin := a.login("admin@a.test")
	analyst := a.login("analyst@a.test")
	org := createOrg(t, a, admin, "preliminary-stop")
	asset := mkAsset(t, a, admin, map[string]any{"category": "web_application", "name": "Scope fixture", "organization_id": org.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.1:8080/"}}})
	response := a.do("POST", "/api/v1/scans", scans.Input{OrganizationID: org.ID, AssetIDs: []uuid.UUID{uuid.MustParse(asset)}}, analyst, "Idempotency-Key", "stop")
	if response.Status != 201 {
		t.Fatalf("launch %d %s", response.Status, response.Body)
	}
	var scan scans.Scan
	response.JSON(&scan)
	opts := server.Options{DB: env.App, Tenant: env.TenantA, Guard: contract.Guard{AllowLoopback: true}}
	svc := server.ScanService(opts)
	rt := server.Runtime(opts)
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	actors := map[string]*execution.Injector{}
	executions := map[string]uuid.UUID{}
	for _, tool := range []string{"nmap", "nuclei"} {
		var id uuid.UUID
		if err := env.Owner.QueryRow(ctx, `SELECT x.id FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1 AND j.tool=$2`, scan.ID, tool).Scan(&id); err != nil {
			t.Fatal(err)
		}
		token, err := execution.IssueToken(ctx, env.App, env.TenantA, tool)
		if err != nil {
			t.Fatal(err)
		}
		actor, err := rt.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rt.Claim(ctx, actor, id); err != nil {
			t.Fatal(err)
		}
		actors[tool] = actor
		executions[tool] = id
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE assets SET testing_authorized=false WHERE id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	for tool, actor := range actors {
		cancel, err := rt.Heartbeat(ctx, actor, executions[tool])
		if err != nil || !cancel {
			t.Fatalf("preliminary %s ignored revocation: %v %v", tool, cancel, err)
		}
	}
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	response = a.do("GET", "/api/v1/scans/"+scan.ID.String(), nil, analyst)
	response.JSON(&scan)
	if scan.Status != "canceled" || hackbotJob(scan.Jobs).Status != "canceled" || hackbotJob(scan.Jobs).SimulationID != nil {
		t.Fatalf("dependent orchis-redteam-agent was left queued or started: %+v", scan)
	}
}

func TestHackbotScannerHandoffStaysWithinAsset(t *testing.T) {
	a, env := setup(t)
	ctx := context.Background()
	admin := a.login("admin@a.test")
	analyst := a.login("analyst@a.test")
	org := createOrg(t, a, admin, "handoff-isolation")
	assets := []uuid.UUID{}
	for _, host := range []string{"127.0.0.1", "127.0.0.2"} {
		assets = append(assets, uuid.MustParse(mkAsset(t, a, admin, map[string]any{"category": "web_application", "name": host, "organization_id": org.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://" + host + ":8080/"}}})))
	}
	response := a.do("POST", "/api/v1/scans", scans.Input{OrganizationID: org.ID, AssetIDs: assets}, analyst, "Idempotency-Key", "isolation")
	if response.Status != 201 {
		t.Fatalf("launch: %d %s", response.Status, response.Body)
	}
	var scan scans.Scan
	response.JSON(&scan)
	opts := server.Options{DB: env.App, Tenant: env.TenantA, Guard: contract.Guard{AllowLoopback: true}}
	svc := server.ScanService(opts)
	rt := server.Runtime(opts)
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	// Only finish the first asset's preliminary scans. Its Hackbot must not wait
	// for another asset, or consume that other asset's findings.
	for _, tool := range []string{"nmap", "nuclei"} {
		var id uuid.UUID
		if err := env.Owner.QueryRow(ctx, `SELECT x.id FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1 AND j.asset_id=$2 AND j.tool=$3`, scan.ID, assets[0], tool).Scan(&id); err != nil {
			t.Fatal(err)
		}
		token, err := execution.IssueToken(ctx, env.App, env.TenantA, tool)
		if err != nil {
			t.Fatal(err)
		}
		actor, err := rt.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = rt.Claim(ctx, actor, id); err != nil {
			t.Fatal(err)
		}
		typ := "vulnerability"
		if tool == "nmap" {
			typ = "open_port"
		}
		code := 0
		if err = rt.Complete(ctx, actor, id, execution.Result{ExitCode: &code, Findings: []execution.NewFinding{{Type: typ, Title: "Only first asset", Value: assets[0].String() + "-" + tool, Severity: "info", Details: json.RawMessage(`{}`)}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT x.id FROM executions x JOIN scan_jobs j ON j.tenant_id=x.tenant_id AND j.simulation_id=x.simulation_id WHERE j.assessment_id=$1 AND j.asset_id=$2 AND j.tool='hackbot'`, scan.ID, assets[0]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	token, err := execution.IssueToken(ctx, env.App, env.TenantA, "hackbot")
	if err != nil {
		t.Fatal(err)
	}
	actor, err := rt.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := rt.Claim(ctx, actor, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.ScannerInputs) != 2 {
		t.Fatal("wrong number of preliminary inputs")
	}
	for _, input := range spec.ScannerInputs {
		if !strings.Contains(string(input.Findings), assets[0].String()) || strings.Contains(string(input.Findings), assets[1].String()) {
			t.Fatal("cross-asset preliminary findings")
		}
	}
	var status string
	if err = env.Owner.QueryRow(ctx, `SELECT status FROM scan_jobs WHERE assessment_id=$1 AND asset_id=$2 AND tool='hackbot'`, scan.ID, assets[1]).Scan(&status); err != nil || status != "waiting" {
		t.Fatal("second asset bypassed prerequisites", status, err)
	}
}
