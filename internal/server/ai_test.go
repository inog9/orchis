package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/ai"
	"orchis-go/internal/contract"
	"orchis-go/internal/reporting"
	"orchis-go/internal/server"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const aiWorkerToken = "local-test-worker-token-32-characters-only"

func aiFixture(t *testing.T) (*scanFixture, uuid.UUID, uuid.UUID, ai.PolicyInput) {
	t.Helper()
	f := newScanFixture(t, true)
	srv := httptest.NewServer(server.New(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, Guard: contract.Guard{AllowLoopback: true}, AIKey: bytes.Repeat([]byte{9}, 32), AIWorkerToken: aiWorkerToken}))
	t.Cleanup(srv.Close)
	f.a.srv = srv
	org := createOrg(t, f.a, f.admin, "ai-private-corp")
	r := f.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", map[string]any{"assets": []any{map[string]any{"id": f.asset, "version": 1}}}, f.admin)
	if r.Status != 204 {
		t.Fatalf("assign %d %s", r.Status, r.Body)
	}
	report, assessment := uuid.New(), uuid.New()
	var user uuid.UUID
	if e := f.a.env.Owner.QueryRow(context.Background(), `SELECT id FROM users WHERE tenant_id=$1 AND email='admin@a.test'`, f.a.env.TenantA).Scan(&user); e != nil {
		t.Fatal(e)
	}
	snap := reporting.Snapshot{Organization: "Private corp", Assets: []reporting.Asset{{ID: uuid.MustParse(f.asset), Name: "private-user@example.test", Status: "finished", Findings: []reporting.Finding{{ID: uuid.NewString(), Title: "PRIVATE password=hunter2", Type: "vulnerability", Severity: "high", Value: "http://user:password@internal/", Evidence: map[string]any{"cve": "CVE-2026-12345", "raw": "API_KEY=do-not-send"}}}}}}
	data, _ := json.Marshal(snap)
	if _, e := f.a.env.Owner.Exec(context.Background(), `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,status,created_by) VALUES($1,$2,$3,'AI source','quick','ai-source','hash','finished',$4)`, assessment, f.a.env.TenantA, org.ID, user); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.env.Owner.Exec(context.Background(), `INSERT INTO reports(id,tenant_id,organization_id,assessment_id,snapshot,status) VALUES($1,$2,$3,$4,$5,'ready')`, report, f.a.env.TenantA, org.ID, assessment, data); e != nil {
		t.Fatal(e)
	}
	pol := ai.PolicyInput{Policy: ai.Policy{Version: 0, Enabled: true, Planning: true, Remediation: true, PlanningModel: "fake-deepseek", RemediationModel: "fake-openai", MaxInputBytes: 64000, MaxOutputTokens: 1000, DailyTokens: 1000000, DailyCalls: 20, DailyCostMicros: 10000000, InputRateMicros: 1000000, OutputRateMicros: 1000000}, DeepSeekKey: "fake-private-deepseek", OpenAIKey: "fake-private-openai", Consent: true}
	return f, org.ID, report, pol
}
func aiRequest(t *testing.T, f *scanFixture, org, report uuid.UUID, key string) ai.Job {
	t.Helper()
	r := f.a.do("POST", "/api/v1/organizations/"+org.String()+"/ai/jobs", ai.Request{ReportID: report, Kind: "planning"}, f.analyst, "Idempotency-Key", key)
	if r.Status != 200 {
		t.Fatalf("request %d %s", r.Status, r.Body)
	}
	var j ai.Job
	r.JSON(&j)
	return j
}
func aiClaim(t *testing.T, f *scanFixture) ai.Claim {
	t.Helper()
	r := f.a.do("POST", "/api/v1/ai-worker/claim", nil, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 200 {
		t.Fatalf("claim %d %s", r.Status, r.Body)
	}
	var c ai.Claim
	r.JSON(&c)
	return c
}
func aiOutput() json.RawMessage {
	return json.RawMessage(`{"summary":"Unverified guidance","remediations":[{"finding_id":"finding_1","recommendation":"Update the affected component","validation":"Repeat the approved scan"}],"actions":[{"asset_id":"asset_1","contract_key":"nmap.tcp_connect","version":1,"params":{"profile":"top100"},"reason":"Review exposure"}]}`)
}
func TestAIConsentTenantBudgetAndIdempotency(t *testing.T) {
	f, org, report, pol := aiFixture(t)
	base := "/api/v1/organizations/" + org.String() + "/ai"
	r := f.a.do("GET", base, nil, f.viewer)
	var before ai.Policy
	r.JSON(&before)
	if r.Status != 200 || before.Enabled || before.HasKeys {
		t.Fatal("AI enabled by default")
	}
	r = f.a.do("POST", base+"/jobs", ai.Request{ReportID: report, Kind: "planning"}, f.analyst, "Idempotency-Key", "disabled")
	if r.Status != 409 {
		t.Fatal("unconsented request accepted", r.Status)
	}
	if r = f.a.do("PUT", base, pol, f.analyst); r.Status != 403 {
		t.Fatal("analyst changed consent")
	}
	pol.Consent = false
	if r = f.a.do("PUT", base, pol, f.admin); r.Status != 409 {
		t.Fatal("missing consent accepted")
	}
	pol.Consent = true
	r = f.a.do("PUT", base, pol, f.admin)
	if r.Status != 200 {
		t.Fatalf("policy %d %s", r.Status, r.Body)
	}
	if strings.Contains(string(r.Body), "fake-private") {
		t.Fatal("credential leaked")
	}
	if r = f.a.do("PUT", base, pol, f.admin); r.Status != 409 {
		t.Fatal("stale policy accepted")
	}
	var cipher []byte
	if e := f.a.env.Owner.QueryRow(context.Background(), `SELECT secrets_enc FROM ai_policies WHERE organization_id=$1`, org).Scan(&cipher); e != nil || bytes.Contains(cipher, []byte("fake-private")) {
		t.Fatal("plaintext credentials", e)
	}
	r = f.a.do("GET", base+"/preview?report_id="+report.String(), nil, f.analyst)
	for _, private := range []string{"Private corp", "private-user", "hunter2", "internal", "do-not-send"} {
		if bytes.Contains(r.Body, []byte(private)) {
			t.Fatal("private data in outgoing preview", private)
		}
	}
	j := aiRequest(t, f, org, report, "same-key")
	again := aiRequest(t, f, org, report, "same-key")
	if again.ID != j.ID {
		t.Fatal("duplicate request")
	}
	r = f.a.do("POST", base+"/jobs", ai.Request{ReportID: report, Kind: "remediation"}, f.analyst, "Idempotency-Key", "same-key")
	if r.Status != 409 {
		t.Fatal("key reused for different request")
	}
	if r = f.a.do("POST", "/api/v1/ai-worker/claim", nil, f.admin); r.Status != 401 {
		t.Fatal("session authorized worker")
	}
	claim := aiClaim(t, f)
	if claim.Job.ID != j.ID || claim.APIKey != "fake-private-deepseek" {
		t.Fatal("wrong claim")
	}
	r = f.a.do("POST", "/api/v1/ai-worker/claim", nil, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 204 {
		t.Fatal("concurrent duplicate claim")
	}
	r = f.a.do("POST", "/api/v1/ai-worker/jobs/"+j.ID.String()+"/complete", ai.Completion{Token: claim.Token, Output: aiOutput(), UsageTokens: 100}, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 204 {
		t.Fatal("complete", r.Status, string(r.Body))
	}
	if r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/approve", map[string]int{"version": 3}, f.viewer); r.Status != 403 {
		t.Fatal("viewer approved active checks")
	}
	r = f.a.do("GET", "/api/v1/ai/jobs/"+j.ID.String(), nil, f.viewer)
	r.JSON(&j)
	var count int
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE atomic_test_id IN(SELECT atomic_test_id FROM ai_job_actions WHERE job_id=$1)`, j.ID).Scan(&count)
	if count != 0 || j.Status != "review" {
		t.Fatal("executed before approval")
	}
	// Pencabutan izin aset setelah antre tetap menggagalkan approval secara atomik.
	f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET testing_authorized=false WHERE id=$1`, f.asset)
	r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/approve", map[string]int{"version": j.Version}, f.analyst)
	if r.Status != 422 {
		t.Fatalf("authorization revoked %d %s", r.Status, r.Body)
	}
	f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET testing_authorized=true WHERE id=$1`, f.asset)
	r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/approve", map[string]int{"version": j.Version}, f.analyst)
	if r.Status != 200 {
		t.Fatalf("approval %d %s", r.Status, r.Body)
	}
	againResp := f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/approve", map[string]int{"version": j.Version}, f.analyst)
	if againResp.Status != 200 {
		t.Fatal("approval idempotency")
	}
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM ai_job_actions WHERE job_id=$1`, j.ID).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate active action")
	}
	r = f.a.do("GET", "/api/v1/ai/jobs/"+j.ID.String(), nil, f.analyst)
	r.JSON(&j)
	r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/cancel", map[string]int{"version": j.Version}, f.analyst)
	if r.Status != 200 {
		t.Fatal("cancel approved actions", r.Status, string(r.Body))
	}
	var outcome string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT e.outcome FROM executions e JOIN ai_job_actions a ON a.tenant_id=e.tenant_id AND a.execution_id=e.id WHERE a.job_id=$1`, j.ID).Scan(&outcome)
	if outcome != "canceled" {
		t.Fatal("active checks were not canceled")
	}
	other := newApp(t, f.a.env, f.a.env.TenantB)
	other.user(f.a.env.TenantB, "ai@b.test", "admin")
	cookie := other.login("ai@b.test")
	if r = other.do("GET", "/api/v1/ai/jobs/"+j.ID.String(), nil, cookie); r.Status != 404 {
		t.Fatal("cross tenant job")
	}
	if r = other.do("GET", base, nil, cookie); r.Status != 404 {
		t.Fatal("cross tenant policy")
	}
	// Anggaran diserialkan oleh row policy; semua reservasi, termasuk hasil final, tetap dihitung.
	pol.Version = 1
	pol.DailyCalls = 1
	r = f.a.do("PUT", base, pol, f.admin)
	if r.Status != 200 {
		t.Fatal("budget policy")
	}
	r = f.a.do("POST", base+"/jobs", ai.Request{ReportID: report, Kind: "planning"}, f.analyst, "Idempotency-Key", "over-budget")
	if r.Status != 409 {
		t.Fatal("budget bypass")
	}
}
func TestAIRevocationCancellationLeaseAndInvalidOutput(t *testing.T) {
	f, org, report, pol := aiFixture(t)
	base := "/api/v1/organizations/" + org.String() + "/ai"
	r := f.a.do("PUT", base, pol, f.admin)
	if r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	j := aiRequest(t, f, org, report, "first")
	c := aiClaim(t, f)
	pol.Version = 1
	pol.Enabled = false
	r = f.a.do("PUT", base, pol, f.admin)
	if r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	r = f.a.do("POST", "/api/v1/ai-worker/jobs/"+j.ID.String()+"/complete", ai.Completion{Token: c.Token, Output: aiOutput()}, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 409 {
		t.Fatal("late output after revoke accepted")
	}
	r = f.a.do("POST", base+"/jobs", ai.Request{ReportID: report, Kind: "planning"}, f.analyst, "Idempotency-Key", "disabled-after-revoke")
	if r.Status != 409 {
		t.Fatal("new request accepted after consent revoked")
	}
	pol.Version = 2
	pol.Enabled = true
	r = f.a.do("PUT", base, pol, f.admin)
	if r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	j = aiRequest(t, f, org, report, "second")
	c = aiClaim(t, f)
	r = f.a.do("POST", "/api/v1/ai-worker/jobs/"+j.ID.String()+"/complete", ai.Completion{Token: c.Token, Output: json.RawMessage(`{"command":"curl evil"}`)}, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 204 {
		t.Fatal(string(r.Body))
	}
	r = f.a.do("GET", "/api/v1/ai/jobs/"+j.ID.String(), nil, f.viewer)
	r.JSON(&j)
	if j.Status != "failed" || strings.Contains(string(r.Body), "curl evil") {
		t.Fatal("invalid model output persisted")
	}
	f.a.env.Owner.Exec(context.Background(), `UPDATE ai_jobs SET updated_at=now()-interval '1 minute' WHERE id=$1`, j.ID)
	j = aiRequest(t, f, org, report, "third")
	c = aiClaim(t, f)
	f.a.env.Owner.Exec(context.Background(), `UPDATE ai_jobs SET lease_until=now()-interval '1 minute' WHERE id=$1`, j.ID)
	r = f.a.do("POST", "/api/v1/ai-worker/claim", nil, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 204 {
		t.Fatal("expired paid request retried")
	}
	r = f.a.do("GET", "/api/v1/ai/jobs/"+j.ID.String(), nil, f.viewer)
	r.JSON(&j)
	if j.Status != "failed" {
		t.Fatal("lost worker not recovered")
	}
	j = aiRequest(t, f, org, report, "fourth")
	r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/cancel", map[string]int{"version": j.Version}, f.analyst)
	if r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/cancel", map[string]int{"version": j.Version}, f.analyst)
	if r.Status != 409 {
		t.Fatal("version not checked")
	}
	// Kehilangan master key tidak boleh menghalangi pencabutan consent darurat.
	emergency := httptest.NewServer(server.New(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, AIKey: []byte{}, AIWorkerToken: aiWorkerToken}))
	defer emergency.Close()
	old := f.a.srv
	f.a.srv = emergency
	pol.Version = 3
	pol.Enabled = false
	pol.Consent = false
	pol.DeepSeekKey = ""
	pol.OpenAIKey = ""
	r = f.a.do("PUT", base, pol, f.admin)
	f.a.srv = old
	if r.Status != 200 {
		t.Fatal("could not revoke after master key loss", r.Status, string(r.Body))
	}

}

// Beberapa worker tidak boleh mengklaim dua panggilan provider untuk organisasi yang sama.
func TestAIConcurrentClaimsAreSerialized(t *testing.T) {
	f, org, report, pol := aiFixture(t)
	r := f.a.do("PUT", "/api/v1/organizations/"+org.String()+"/ai", pol, f.admin)
	if r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	aiRequest(t, f, org, report, "parallel-one")
	aiRequest(t, f, org, report, "parallel-two")
	svc := server.AIService(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, AIKey: bytes.Repeat([]byte{9}, 32), AIWorkerToken: aiWorkerToken})
	for round := 0; round < 10; round++ {
		if _, e := f.a.env.Owner.Exec(context.Background(), `UPDATE ai_jobs SET status='queued',claim_hash=NULL,lease_until=NULL WHERE organization_id=$1`, org); e != nil {
			t.Fatal(e)
		}
		start := make(chan struct{})
		results := make(chan bool, 8)
		errs := make(chan error, 8)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				c, e := svc.Claim(context.Background())
				if e != nil {
					errs <- e
				}
				results <- c != nil
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			t.Fatal(e)
		}
		claimed := 0
		for v := range results {
			if v {
				claimed++
			}
		}
		if claimed != 1 {
			t.Fatalf("round %d: %d simultaneous claims", round, claimed)
		}
	}
}

func TestAIAdviceWithoutActionsDoesNotClaimChecksWereQueued(t *testing.T) {
	f, org, report, pol := aiFixture(t)
	r := f.a.do("PUT", "/api/v1/organizations/"+org.String()+"/ai", pol, f.admin)
	if r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	j := aiRequest(t, f, org, report, "advice-only")
	c := aiClaim(t, f)
	out := ai.Output{Summary: "No additional check justified", Actions: []ai.Action{}, Remediations: []ai.Advice{{FindingID: "finding_1", Recommendation: "Review the observed component", Validation: "Compare against the source report"}}}
	b, _ := json.Marshal(out)
	r = f.a.do("POST", "/api/v1/ai-worker/jobs/"+j.ID.String()+"/complete", ai.Completion{Token: c.Token, Output: b, UsageTokens: 100}, "", "Authorization", "Bearer "+aiWorkerToken)
	if r.Status != 204 {
		t.Fatal(string(r.Body))
	}
	r = f.a.do("GET", "/api/v1/ai/jobs/"+j.ID.String(), nil, f.viewer)
	r.JSON(&j)
	if j.Status != "completed" {
		t.Fatal("empty plan incorrectly requests active approval")
	}
	r = f.a.do("POST", "/api/v1/ai/jobs/"+j.ID.String()+"/approve", map[string]int{"version": j.Version}, f.analyst)
	if r.Status != 409 {
		t.Fatal("advice-only job allowed dispatch")
	}
}

func TestAIAdvisoryPDFUsesConfiguredRemoteRenderer(t *testing.T) {
	const token = "local-renderer-fixture-token-32-characters"
	var calls atomic.Int32
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing renderer authentication")
			w.WriteHeader(401)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Contains(body, []byte("AI advisory supplement")) {
			t.Error("missing AI print HTML")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4\nLOCAL transport fixture; not a rendered document"))
	}))
	defer renderer.Close()
	t.Setenv("ORCHIS_RENDERER_URL", renderer.URL)
	t.Setenv("ORCHIS_RENDERER_TOKEN", token)
	t.Setenv("ORCHIS_REPORT_RENDER_SCRIPT", "/missing-renderer-local-script-must-not-run.mjs")
	f, org, report, pol := aiFixture(t)
	if r := f.a.do("PUT", "/api/v1/organizations/"+org.String()+"/ai", pol, f.admin); r.Status != 200 {
		t.Fatalf("policy: %d", r.Status)
	}
	job := aiRequest(t, f, org, report, "remote-pdf")
	claim := aiClaim(t, f)
	if r := f.a.do("POST", "/api/v1/ai-worker/jobs/"+job.ID.String()+"/complete", ai.Completion{Token: claim.Token, Output: aiOutput(), UsageTokens: 100}, "", "Authorization", "Bearer "+aiWorkerToken); r.Status != 204 {
		t.Fatalf("fixture completion: %d", r.Status)
	}
	response := f.a.do("GET", "/api/v1/ai/jobs/"+job.ID.String()+"/export/pdf", nil, f.analyst)
	if response.Status != 200 || !bytes.HasPrefix(response.Body, []byte("%PDF-")) || calls.Load() != 1 {
		t.Fatalf("remote AI renderer not used: status=%d calls=%d", response.Status, calls.Load())
	}
}
