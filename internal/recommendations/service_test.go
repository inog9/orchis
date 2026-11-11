package recommendations

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/testutil"
	"testing"
)

func fixture(t *testing.T, env *testutil.Env, status string, findings []reporting.Finding) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	org, scan, actor, report := ids.New(), ids.New(), ids.New(), ids.New()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := env.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'Review fixture',$3)`, org, env.TenantA, org.String())
	must(`INSERT INTO users(id,tenant_id,email,display_name,role_id) SELECT $1,$2,$3,'Fixture',id FROM roles WHERE tenant_id=$2 AND key='admin'`, actor, env.TenantA, actor.String()+"@fixture.test")
	must(`INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by,is_scan,status) VALUES($1,$2,$3,'Review fixture','quick',$5,'fixture',$4,true,$6)`, scan, env.TenantA, org, actor, scan.String(), status)
	raw, _ := json.Marshal(reporting.Snapshot{Language: "id", Status: status, Assets: []reporting.Asset{{Findings: findings}}})
	must(`INSERT INTO reports(id,tenant_id,organization_id,assessment_id,snapshot) VALUES($1,$2,$3,$4,$5)`, report, env.TenantA, org, scan, raw)
	for i := 0; i < 2; i++ {
		if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error { return Enqueue(ctx, tx, env.TenantA, scan) }); err != nil {
			t.Fatal(err)
		}
	}
	return scan
}
func TestDurableReviewClaimIsolationAndCompletion(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	svc := Service{DB: env.App, Tenant: env.TenantA}
	findings := []reporting.Finding{{ID: "f1", Title: "Injection", Type: "vulnerability", Severity: "high"}}
	scan := fixture(t, env, "incomplete", findings)
	other := fixture(t, env, "finished", findings)
	job, err := svc.Get(ctx, env.TenantA, scan)
	if err != nil || job == nil || job.Status != "queued" || job.Input.Language != "id" {
		t.Fatal(job, err)
	}
	if hidden, err := svc.Get(ctx, env.TenantB, scan); err != nil || hidden != nil {
		t.Fatalf("cross tenant leak: %v %v", hidden, err)
	}
	c, err := svc.Claim(ctx)
	if err != nil || c == nil {
		t.Fatal(c, err)
	}
	if next, err := svc.Claim(ctx); err != nil || next != nil {
		t.Fatalf("parallel model call: %v %v", next, err)
	}
	good := json.RawMessage(`{"summary":"Fix the recorded issue.","items":[{"finding_id":"f1","priority":"now","reason":"High severity.","action":"Fix the injection boundary.","validation":"Retest the affected function."}]}`)
	if err = svc.Complete(ctx, c.ID, Completion{Token: "wrong", Output: good}); err == nil {
		t.Fatal("wrong claim token accepted")
	}
	if err = svc.Complete(ctx, c.ID, Completion{Token: c.Token, Output: good}); err != nil {
		t.Fatal(err)
	}
	if err = svc.Complete(ctx, c.ID, Completion{Token: c.Token, Output: good}); err == nil {
		t.Fatal("completed output was overwritten")
	}
	var n int
	if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM scan_recommendations WHERE scan_id=$1`, scan).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	c, err = svc.Claim(ctx)
	if err != nil || c == nil {
		t.Fatal(c, err)
	}
	if _, err = env.Owner.Exec(ctx, `UPDATE scan_recommendations SET lease_until=now()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if next, err := svc.Claim(ctx); err != nil || next != nil {
		t.Fatalf("interrupted model call replayed: %v %v", next, err)
	}
	for _, id := range []uuid.UUID{scan, other} {
		j, e := svc.Get(ctx, env.TenantA, id)
		if e != nil || j == nil || j.Status == "queued" || j.Status == "running" {
			t.Fatal(j, e)
		}
	}
	if err = svc.Complete(ctx, c.ID, Completion{Token: c.Token, Output: good}); err == nil {
		t.Fatal("late result accepted")
	}
}
func TestNoFindingsAndExpiredEvidenceDoNotTriggerModelCalls(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	svc := Service{DB: env.App, Tenant: env.TenantA}
	scan := fixture(t, env, "finished", nil)
	j, err := svc.Get(ctx, env.TenantA, scan)
	if err != nil || j.Status != "not_needed" {
		t.Fatal(j, err)
	}
	expired := fixture(t, env, "finished", []reporting.Finding{{ID: "f1", Type: "vulnerability", Severity: "high"}})
	if _, err = env.Owner.Exec(ctx, `UPDATE assessments SET results_expired_at=now() WHERE id=$1`, expired); err != nil {
		t.Fatal(err)
	}
	if c, err := svc.Claim(ctx); err != nil || c != nil {
		t.Fatal("expired scan was claimed", c, err)
	}
	if j, err := svc.Get(ctx, env.TenantA, expired); err != nil || j != nil {
		t.Fatal("expired advice exposed", j, err)
	}
	if _, err = env.Owner.Exec(ctx, `DELETE FROM reports WHERE assessment_id=$1`, expired); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM scan_recommendations WHERE scan_id=$1`, expired).Scan(&n); err != nil || n != 0 {
		t.Fatal("retention did not delete review", n, err)
	}
}
