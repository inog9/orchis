package retention

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/testutil"
	"testing"
	"time"
)

func TestSweepIndependentLifetimesClockIsolationAndRestart(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	pb := admin(t, e, e.TenantB)
	a, b := fixture(t, e, p), fixture(t, e, pb)
	now := time.Date(2028, 2, 1, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []orgFixture{a, b} {
		must(`UPDATE assessments SET created_at=$2,ended_at=$2 WHERE id=$1`, f.assessment, old)
		must(`UPDATE reports SET created_at=$2,status='ready' WHERE id=$1`, f.report, old)
		must(`UPDATE executions SET created_at=$2,ended_at=$2 WHERE id=$1`, f.execution, old)
		must(`UPDATE findings SET first_seen_at=$2,last_seen_at=$2 WHERE id=$1`, f.finding, old)
		must(`UPDATE endpoint_datasets SET collected_at=$2,last_attempt_at=$3,last_status='failed' WHERE asset_id=$1`, f.asset, old, now)
	}
	s := Service{DB: e.App, Now: func() time.Time { return now }}
	policy := Policy{FindingsDays: 1, ScanDays: 1, ReportDays: 3, EndpointDays: 1, AuditDays: 90}
	if _, err := s.Update(ctx, p, policy, "", "fixture"); err != nil {
		t.Fatal(err)
	}
	expiredAudit, recentAudit := uuid.New(), uuid.New()
	must(`INSERT INTO audit_log(id,tenant_id,at,action) VALUES($1,$2,$3,'fixture'),($4,$2,$5,'fixture')`, expiredAudit, p.TenantID, now.Add(-91*24*time.Hour), recentAudit, now.Add(-89*24*time.Hour))
	if err := s.Sweep(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"findings", "executions"} {
		id := a.finding
		if table == "executions" {
			id = a.execution
		}
		if count(`SELECT count(*) FROM `+table+` WHERE id=$1`, id) != 0 {
			t.Fatal("expired row remains", table)
		}
	}
	if count(`SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1`, a.asset) != 0 {
		t.Fatal("failed attempt extended old successful dataset")
	}
	if count(`SELECT count(*) FROM reports WHERE id=$1`, a.report) != 1 {
		t.Fatal("scan expiry removed retained snapshot")
	}
	if count(`SELECT count(*) FROM assessments WHERE id=$1 AND results_expired_at=$2`, a.assessment, now) != 1 {
		t.Fatal("retained report lost explicit expired-results stub")
	}
	if count(`SELECT count(*) FROM organizations WHERE id=$1 AND findings_epoch=1`, a.org) != 1 {
		t.Fatal("finding expiry did not reset comparison identity")
	}
	if count(`SELECT count(*) FROM audit_log WHERE id=$1`, expiredAudit) != 0 || count(`SELECT count(*) FROM audit_log WHERE id=$1`, recentAudit) != 1 {
		t.Fatal("audit boundary wrong")
	}
	for _, table := range []string{"reports", "findings", "executions", "assessments"} {
		id := map[string]uuid.UUID{"reports": b.report, "findings": b.finding, "executions": b.execution, "assessments": b.assessment}[table]
		if count(`SELECT count(*) FROM `+table+` WHERE id=$1`, id) != 1 {
			t.Fatal("foreign retention leak", table)
		}
	}
	if count(`SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1`, b.asset) != 1 {
		t.Fatal("foreign endpoint removed")
	}
	if err := s.Sweep(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM organizations WHERE id=$1 AND findings_epoch=1`, a.org) != 1 {
		t.Fatal("retry changed identity without finding deletion")
	}
	if count(`SELECT count(*) FROM data_cleanup_tasks WHERE tenant_id=$1`, p.TenantID) != 2 {
		t.Fatal("raw output/search durable tasks missing")
	}
	got, err := s.Get(ctx, p)
	if err != nil || got.LastSweepAt == nil || !got.LastSweepAt.Equal(now) {
		t.Fatalf("injected sweep time: %+v %v", got, err)
	}
	s.Now = func() time.Time { return now.Add(48 * time.Hour) }
	if err = s.Sweep(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM reports WHERE id=$1`, a.report) != 0 || count(`SELECT count(*) FROM assessments WHERE id=$1`, a.assessment) != 0 {
		t.Fatal("report expiry did not remove JSON and unnecessary stub")
	}
	if count(`SELECT count(*) FROM data_cleanup_tasks WHERE tenant_id=$1`, p.TenantID) != 3 {
		t.Fatal("PDF cleanup missing")
	}
	c := &cleaner{}
	restarted := Service{DB: e.App, Blob: c, Search: c}
	if err = restarted.CleanupOnce(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM data_cleanup_tasks WHERE tenant_id=$1`, p.TenantID) != 0 || len(c.keys) != 2 || len(c.ids) != 1 {
		t.Fatal("restart did not finish exact artifacts")
	}
}

func TestSweepPinsRenderingActiveRunsAndRecentEndpoint(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a, b := fixture(t, e, p), fixture(t, e, p)
	now := time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC)
	old := now.Add(-400 * 24 * time.Hour)
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`UPDATE reports SET created_at=$2,status='rendering' WHERE id=$1`, a.report, old)
	must(`UPDATE assessments SET created_at=$2,status='running' WHERE id=$1`, b.assessment, old)
	must(`UPDATE reports SET created_at=$2 WHERE id=$1`, b.report, now)
	must(`UPDATE endpoint_datasets SET collected_at=$2,last_attempt_at=$3 WHERE asset_id=$1`, a.asset, now.Add(-30*24*time.Hour), now)
	must(`UPDATE endpoint_datasets SET collected_at=NULL,last_attempt_at=$2 WHERE asset_id=$1`, b.asset, now)
	s := Service{DB: e.App, Now: func() time.Time { return now }}
	if err := s.Sweep(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM reports WHERE id=$1 AND status='rendering'`, a.report).Scan(&n); err != nil || n != 1 {
		t.Fatal("active renderer deleted", err)
	}
	if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM assessments WHERE id=$1 AND results_expired_at IS NULL`, b.assessment).Scan(&n); err != nil || n != 1 {
		t.Fatal("running scan expired", err)
	}
	if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM endpoint_datasets WHERE asset_id=ANY($1::uuid[])`, []uuid.UUID{a.asset, b.asset}).Scan(&n); err != nil || n != 2 {
		t.Fatal("boundary/recent endpoint expired", err)
	}
}
