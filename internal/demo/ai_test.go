package demo_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"orchis-go/internal/ai"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/demo"
	"orchis-go/internal/testutil"
)

func TestAIAndAtomicFixturesAreMarkedValidatedInertAndPurgeable(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App, Blob: emptyArtifacts{}, Search: emptyArtifacts{}}
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "ai"); err != nil {
		t.Fatal(err)
	}
	status, err := svc.Status(ctx, env.TenantA)
	if err != nil {
		t.Fatal(err)
	}
	if status.FormatVersion != 3 || status.Counts["ai_policies"] != 5 || status.Counts["ai_templates"] != 4 || status.Counts["ai_jobs"] != 20 {
		t.Fatalf("missing AI fixtures: %+v", status)
	}
	if n := count(t, env, `SELECT count(*) FROM ai_policies WHERE tenant_id=$1 AND (NOT is_demo OR enabled OR secrets_enc IS NOT NULL OR consent_by IS NULL OR consent_at IS NULL)`, env.TenantA); n != 0 {
		t.Fatal("fixture policy is active, unmarked or lacks synthetic consent provenance")
	}
	if n := count(t, env, `SELECT count(*) FROM executions WHERE tenant_id=$1 AND atomic_test_id IS NOT NULL AND is_demo AND state='ended'`, env.TenantA); n != 3 {
		t.Fatal("missing saved-test results")
	}
	rows, err := env.Owner.Query(ctx, `SELECT input,output,kind,status FROM ai_jobs WHERE tenant_id=$1 AND is_demo`, env.TenantA)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]int{}
	for rows.Next() {
		var input, output []byte
		var kind, state string
		if err := rows.Scan(&input, &output, &kind, &state); err != nil {
			t.Fatal(err)
		}
		var in ai.Input
		if err := json.Unmarshal(input, &in); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(input), "127.0.0.1") || strings.Contains(string(input), "example.invalid") {
			t.Fatal("unminimized fixture input")
		}
		out, err := ai.ValidateOutput(output, in, kind)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Summary, "local fake API") {
			t.Fatal("provider provenance omitted")
		}
		states[state]++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"review", "completed", "rejected", "canceled"} {
		if states[state] != 5 {
			t.Fatalf("missing %s state: %+v", state, states)
		}
	}
	if n := count(t, env, `SELECT count(*) FROM ai_jobs j JOIN ai_templates t ON t.tenant_id=j.tenant_id AND t.id=j.template_id WHERE j.tenant_id=$1 AND j.is_demo AND (j.prompt_version<>t.version OR right(j.instructions,length(t.instructions))<>t.instructions)`, env.TenantA); n != 0 {
		t.Fatal("job prompt differs from its recorded template version")
	}
	var org, job uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT organization_id,id FROM ai_jobs WHERE tenant_id=$1 AND status='review' AND is_demo LIMIT 1`, env.TenantA).Scan(&org, &job); err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{TenantID: env.TenantA, Permissions: []string{authz.AtomicWrite, authz.AtomicRun, authz.AssetRead}}
	service := &ai.Service{DB: env.App, Tenant: env.TenantA, Key: make([]byte, 32), WorkerToken: strings.Repeat("synthetic", 5)}
	policy, err := service.Policy(ctx, p, org)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Enabled || policy.Configured || !policy.IsDemo {
		t.Fatalf("demo policy presented as live: %+v", policy)
	}
	if _, err := service.SavePolicy(ctx, p, org, ai.PolicyInput{Policy: policy}); err == nil {
		t.Fatal("synthetic policy became editable")
	}
	// A manual DB edit cannot make the fixture executable, either.
	if _, err := env.Owner.Exec(ctx, `UPDATE ai_policies SET enabled=true WHERE tenant_id=$1 AND organization_id=$2`, env.TenantA, org); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Approve(ctx, p, job, 1); err == nil || !strings.Contains(err.Error(), "Synthetic AI plans") {
		t.Fatalf("synthetic AI plan lacked explicit refusal: %v", err)
	}
	if _, err := service.Cancel(ctx, p, job, 1); err == nil {
		t.Fatal("synthetic history was mutated")
	}
	if claim, err := service.Claim(ctx); err != nil || claim != nil {
		t.Fatalf("fixture reached a provider worker: %+v %v", claim, err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE ai_jobs SET status='queued' WHERE tenant_id=$1 AND id=$2`, env.TenantA, job); err == nil {
		t.Fatal("synthetic AI became queueable")
	}
	// Simulate a restored legacy/corrupt fixture without the newer constraint.
	// SQL filters must still keep it away from workers, without any provider call.
	if _, err := env.Owner.Exec(ctx, `ALTER TABLE ai_jobs DROP CONSTRAINT synthetic_ai_inert`); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE ai_jobs SET updated_at=now()-interval '1 minute' WHERE tenant_id=$1`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE ai_jobs SET status='queued' WHERE tenant_id=$1 AND id=$2`, env.TenantA, job); err != nil {
		t.Fatal(err)
	}
	if claim, err := service.Claim(ctx); err != nil || claim != nil {
		t.Fatalf("restored demo reached provider worker: %+v %v", claim, err)
	}
	if n := count(t, env, `SELECT count(*) FROM outbox WHERE tenant_id=$1`, env.TenantA); n != 0 {
		t.Fatal("synthetic AI dispatched a scanner")
	}
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "again"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, env, `SELECT count(*) FROM ai_jobs WHERE tenant_id=$1`, env.TenantA); n != 20 {
		t.Fatal("AI fixtures duplicated")
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "purge"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"ai_policies", "ai_templates", "ai_jobs", "ai_job_actions", "executions", "finding_occurrences"} {
		if n := count(t, env, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, env.TenantA); n != 0 {
			t.Fatalf("orphan %s: %d", table, n)
		}
	}
}
