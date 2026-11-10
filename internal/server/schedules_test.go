package server_test

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
	"orchis-go/internal/schedules"
	"orchis-go/internal/server"
	"strings"
	"sync"
	"testing"
	"time"
)

func scheduleInput(c *controlledScan) schedules.Input {
	return schedules.Input{RequestKey: uuid.New(), Name: "Weekly perimeter audit", OrganizationID: c.org, Config: schedules.Config{Rule: schedules.Rule{Frequency: "weekly", Timezone: "Asia/Jakarta", Time: "02:00", Weekday: 1, MonthDay: 1}, Kind: "web", AssetIDs: c.input.AssetIDs, Language: "en", MaxRuntimeMinutes: 360, NotifyChanges: true, NotifyGaps: true, WeeklyDigest: true}}
}
func createSchedule(t *testing.T, c *controlledScan) schedules.Schedule {
	t.Helper()
	r := c.f.a.do("POST", "/api/v1/audit-schedules", scheduleInput(c), c.f.analyst)
	if r.Status != 201 {
		t.Fatalf("schedule create %d %s", r.Status, r.Body)
	}
	var v schedules.Schedule
	r.JSON(&v)
	return v
}
func dueSchedule(t *testing.T, c *controlledScan, id uuid.UUID, ago time.Duration) {
	t.Helper()
	now := time.Unix(0, c.clock.Load())
	if _, e := c.f.a.env.Owner.Exec(context.Background(), `UPDATE audit_schedules SET next_run_at=$2 WHERE id=$1`, id, now.Add(-ago)); e != nil {
		t.Fatal(e)
	}
}
func scheduledRuns(t *testing.T, c *controlledScan, id uuid.UUID) []schedules.Run {
	t.Helper()
	r := c.f.a.do("GET", "/api/v1/audit-schedules/"+id.String()+"/runs", nil, c.f.analyst)
	if r.Status != 200 {
		t.Fatalf("runs %d %s", r.Status, r.Body)
	}
	var v struct{ Items []schedules.Run }
	r.JSON(&v)
	return v.Items
}
func TestAuditScheduleCRUDPermissionsAndTenantIsolation(t *testing.T) {
	c := controlled(t)
	s := createSchedule(t, c)
	if len(s.NextRuns) != 3 || s.Paused || s.RunAs == "" {
		t.Fatalf("missing preview or owner %+v", s)
	}
	in := scheduleInput(c)
	in.OrganizationID = uuid.New()
	if r := c.f.a.do("POST", "/api/v1/audit-schedules", in, c.f.analyst); r.Status != 404 {
		t.Fatalf("foreign org %d", r.Status)
	}
	in = scheduleInput(c)
	in.Config.AssetIDs = []uuid.UUID{uuid.New()}
	if r := c.f.a.do("POST", "/api/v1/audit-schedules", in, c.f.analyst); r.Status != 422 {
		t.Fatalf("foreign asset %d", r.Status)
	}
	if r := c.f.a.do("GET", "/api/v1/audit-schedules", nil, c.f.viewer); r.Status != 403 {
		t.Fatalf("viewer %d", r.Status)
	}
	c.f.a.user(c.f.a.env.TenantB, "scheduler@b.test", "admin")
	b := newApp(t, c.f.a.env, c.f.a.env.TenantB)
	cookie := b.login("scheduler@b.test")
	if r := b.do("GET", "/api/v1/audit-schedules/"+s.ID.String(), nil, cookie); r.Status != 404 {
		t.Fatalf("cross-tenant detail %d", r.Status)
	}
	r := b.do("GET", "/api/v1/audit-schedules", nil, cookie)
	var list struct{ Items []schedules.Schedule }
	r.JSON(&list)
	if r.Status != 200 || len(list.Items) != 0 {
		t.Fatalf("tenant list leaked %d %s", r.Status, r.Body)
	}
	path := "/api/v1/audit-schedules/" + s.ID.String()
	if r = c.f.a.do("POST", path+"/pause", map[string]any{"paused": true, "version": s.Version - 1}, c.f.analyst); r.Status != 409 {
		t.Fatalf("stale update %d", r.Status)
	}
	// Revoked scope must not prevent pausing or deleting a schedule.
	c.f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET testing_authorized=false WHERE id=$1`, c.input.AssetIDs[0])
	r = c.f.a.do("POST", path+"/pause", map[string]any{"paused": true, "version": s.Version}, c.f.analyst)
	if r.Status != 200 {
		t.Fatalf("pause %d %s", r.Status, r.Body)
	}
	r.JSON(&s)
	if !s.Paused || len(s.NextRuns) != 0 {
		t.Fatal("pause retained preview")
	}
	r = c.f.a.do("DELETE", path, map[string]any{"version": s.Version}, c.f.analyst)
	if r.Status != 204 {
		t.Fatalf("delete %d %s", r.Status, r.Body)
	}
	if r = c.f.a.do("GET", path, nil, c.f.analyst); r.Status != 404 {
		t.Fatal("deleted visible")
	}
}
func TestAuditScheduleConcurrentDispatchAndCrashRecovery(t *testing.T) {
	c := controlled(t)
	s := createSchedule(t, c)
	dueSchedule(t, c, s.ID, time.Minute)
	ctx := context.Background()
	svc := server.ScheduleService(c.options)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- svc.Tick(ctx, c.f.a.env.TenantA) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	runs := scheduledRuns(t, c, s.ID)
	if len(runs) != 1 || runs[0].Status != "running" || runs[0].ScanID == nil {
		t.Fatalf("unexpected runs %+v", runs)
	}
	id := *runs[0].ScanID
	// Emulate death after Launch committed but before the occurrence was linked.
	if _, e := c.f.a.env.Owner.Exec(ctx, `UPDATE audit_schedule_runs SET scan_id=NULL,status='queued',started_at=NULL WHERE id=$1`, runs[0].ID); e != nil {
		t.Fatal(e)
	}
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	runs = scheduledRuns(t, c, s.ID)
	if runs[0].ScanID == nil || *runs[0].ScanID != id {
		t.Fatal("recovery duplicated scan")
	}
	var count int
	if e := c.f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM assessments WHERE idempotency_key=$1`, "schedule:"+runs[0].ID.String()).Scan(&count); e != nil || count != 1 {
		t.Fatalf("duplicate scans %d %v", count, e)
	}
	// The same target cannot be launched manually while its scheduled audit runs.
	r := c.f.a.do("POST", "/api/v1/scans", c.input, c.f.analyst, "Idempotency-Key", "manual-overlap")
	if r.Status != 409 {
		t.Fatalf("overlap %d %s", r.Status, r.Body)
	}
}
func TestAuditScheduleMissedWindowAndAlertDedupe(t *testing.T) {
	c := controlled(t)
	s := createSchedule(t, c)
	ctx := context.Background()
	svc := server.ScheduleService(c.options)
	for i := 0; i < 2; i++ {
		dueSchedule(t, c, s.ID, time.Duration(20+i)*time.Minute)
		if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
			t.Fatal(e)
		}
	}
	runs := scheduledRuns(t, c, s.ID)
	if len(runs) != 2 || runs[0].Status != "missed" || runs[0].ScanID != nil {
		t.Fatalf("missed %+v", runs)
	}
	var count int
	if e := c.f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM alerts WHERE kind='scheduled_audit_gap' AND resource_id=$1`, s.ID).Scan(&count); e != nil || count != 1 {
		t.Fatalf("duplicate gap alerts %d %v", count, e)
	}
	r := c.f.a.do("GET", "/api/v1/alerts", nil, c.f.analyst)
	if r.Status != 200 {
		t.Fatalf("inbox %d %s", r.Status, r.Body)
	}
}
func TestAuditScheduleRechecksOwnerAndAuthorisation(t *testing.T) {
	for _, change := range []string{"authorisation", "owner", "permissions", "emergency"} {
		t.Run(change, func(t *testing.T) {
			c := controlled(t)
			s := createSchedule(t, c)
			ctx := context.Background()
			dueSchedule(t, c, s.ID, time.Minute)
			switch change {
			case "authorisation":
				c.f.a.env.Owner.Exec(ctx, `UPDATE assets SET testing_authorized=false WHERE id=$1`, c.input.AssetIDs[0])
			case "permissions":
				c.f.a.env.Owner.Exec(ctx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE tenant_id=users.tenant_id AND key='viewer') WHERE id=(SELECT run_as FROM audit_schedules WHERE id=$1)`, s.ID)
			case "owner":
				c.f.a.env.Owner.Exec(ctx, `UPDATE users SET status='disabled' WHERE id=(SELECT run_as FROM audit_schedules WHERE id=$1)`, s.ID)
			case "emergency":
			}
			if change == "emergency" { // use the existing tested organisation hold API
				c.window(t, 0)
				c.clock.Store(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC).UnixNano())
				dueSchedule(t, c, s.ID, time.Minute)
			}
			if e := server.ScheduleService(c.options).Tick(ctx, c.f.a.env.TenantA); e != nil {
				t.Fatal(e)
			}
			var status string
			var scan *uuid.UUID
			if e := c.f.a.env.Owner.QueryRow(ctx, `SELECT status,scan_id FROM audit_schedule_runs WHERE schedule_id=$1`, s.ID).Scan(&status, &scan); e != nil {
				t.Fatal(e)
			}
			if status != "blocked" || scan != nil {
				t.Fatalf("unsafe launch %s %v", status, scan)
			}
		})
	}
}
func TestAuditScheduleRuntimeLimitAndHistory(t *testing.T) {
	c := controlled(t)
	s := createSchedule(t, c)
	ctx := context.Background()
	dueSchedule(t, c, s.ID, time.Minute)
	svc := server.ScheduleService(c.options)
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	runs := scheduledRuns(t, c, s.ID)
	if len(runs) != 1 || runs[0].ScanID == nil {
		t.Fatal("not launched")
	}
	if _, e := c.f.a.env.Owner.Exec(ctx, `UPDATE audit_schedule_runs SET started_at=$2 WHERE id=$1`, runs[0].ID, time.Unix(0, c.clock.Load())); e != nil {
		t.Fatal(e)
	}
	c.clock.Add(int64(7 * time.Hour))
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	runs = scheduledRuns(t, c, s.ID)
	if runs[0].Status != "canceled" || runs[0].Reason == "" || runs[0].Summary.Completed != 0 || runs[0].Summary.Resolved != 0 {
		t.Fatalf("timeout %+v", runs[0])
	}
}
func TestAuditScheduleDynamicAssetsAndResumeNoBacklog(t *testing.T) {
	c := controlled(t)
	in := scheduleInput(c)
	in.Config.AllAssets = true
	in.Config.AssetIDs = []uuid.UUID{}
	r := c.f.a.do("POST", "/api/v1/audit-schedules", in, c.f.analyst)
	if r.Status != 201 {
		t.Fatalf("create %d %s", r.Status, r.Body)
	}
	var s schedules.Schedule
	r.JSON(&s)
	second := mkAsset(t, c.f.a, c.f.admin, map[string]any{"category": "web_application", "name": "New authorised asset", "organization_id": c.org, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.2:8080/"}}})
	dueSchedule(t, c, s.ID, time.Minute)
	if e := server.ScheduleService(c.options).Tick(context.Background(), c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	var input scans.Input
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT input FROM audit_schedule_runs WHERE schedule_id=$1`, s.ID).Scan(&input); e != nil {
		t.Fatal(e)
	}
	if len(input.AssetIDs) != 2 {
		t.Fatalf("new asset %s missing: %+v", second, input.AssetIDs)
	}
	path := "/api/v1/audit-schedules/" + s.ID.String() + "/pause"
	r = c.f.a.do("POST", path, map[string]any{"paused": true, "version": s.Version}, c.f.analyst)
	r.JSON(&s)
	c.clock.Add(int64(30 * 24 * time.Hour))
	r = c.f.a.do("POST", path, map[string]any{"paused": false, "version": s.Version}, c.f.analyst)
	if r.Status != 200 {
		t.Fatalf("resume %d %s", r.Status, r.Body)
	}
	r.JSON(&s)
	if !s.NextRunAt.After(time.Unix(0, c.clock.Load())) {
		t.Fatal("replayed backlog")
	}
}

func TestAuditScheduleChangeAlertsReappearanceAndWeeklyDigest(t *testing.T) {
	c := controlled(t)
	ctx := context.Background()
	s := createSchedule(t, c)
	svc := server.ScheduleService(c.options)
	complete := func(scan uuid.UUID, snap reporting.Snapshot) {
		t.Helper()
		if _, e := c.f.a.env.Owner.Exec(ctx, `UPDATE scan_jobs SET status='finished',ended_at=now() WHERE assessment_id=$1`, scan); e != nil {
			t.Fatal(e)
		}
		if e := server.ScanService(c.options).Tick(ctx, c.f.a.env.TenantA); e != nil {
			t.Fatal(e)
		}
		if _, e := c.f.a.env.Owner.Exec(ctx, `UPDATE reports SET snapshot=$2 WHERE assessment_id=$1`, scan, snap); e != nil {
			t.Fatal(e)
		}
	}
	dueSchedule(t, c, s.ID, time.Minute)
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	first := scheduledRuns(t, c, s.ID)[0]
	old := reporting.Snapshot{AssessmentID: *first.ScanID, FindingsEpoch: 99, CreatedAt: first.StartedAt.Add(-time.Minute), Assets: []reporting.Asset{{ID: c.input.AssetIDs[0], Status: "finished", Methods: []reporting.Method{{Outcome: "success"}}, Findings: []reporting.Finding{{ID: "stable-finding", Type: "vulnerability", Severity: "critical", Title: "CONFIDENTIAL EVIDENCE MUST STAY IN APP"}}}}}
	complete(*first.ScanID, old)
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	c.clock.Add(int64(time.Hour))
	dueSchedule(t, c, s.ID, time.Minute)
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	second := scheduledRuns(t, c, s.ID)[0]
	fresh := old
	fresh.AssessmentID = *second.ScanID
	fresh.CreatedAt = *second.StartedAt
	fresh.ComparisonDetails = &reporting.Comparison{Totals: reporting.ChangeCounts{New: 1}, Rows: []reporting.FindingChange{{AssetID: c.input.AssetIDs[0], FindingID: "stable-finding", Type: "vulnerability", Severity: "critical", Change: "new"}}}
	complete(*second.ScanID, fresh)
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	runs := scheduledRuns(t, c, s.ID)
	if runs[0].Summary.Reappeared != 1 || runs[0].Summary.New != 0 || runs[0].Summary.Attention != 1 || runs[0].Summary.Completed != 1 {
		t.Fatalf("comparison summary %+v", runs[0])
	}
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := c.f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM alerts WHERE kind='scheduled_audit_changes' AND resource_id=$1`, second.ScanID).Scan(&n); e != nil || n != 1 {
		t.Fatalf("change deduplication %d %v", n, e)
	}
	if _, e := c.f.a.env.Owner.Exec(ctx, `UPDATE audit_schedules SET next_digest_at=$2 WHERE id=$1`, s.ID, time.Unix(0, c.clock.Load()).Add(-time.Minute)); e != nil {
		t.Fatal(e)
	}
	if e := svc.Tick(ctx, c.f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if e := c.f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM alerts WHERE kind='scheduled_audit_digest' AND resource_id=$1`, s.ID).Scan(&n); e != nil || n != 1 {
		t.Fatalf("digest %d %v", n, e)
	}
	r := c.f.a.do("GET", "/api/v1/alerts", nil, c.f.analyst)
	if r.Status != 200 || !strings.Contains(string(r.Body), "scheduled_audit_digest") || !strings.Contains(string(r.Body), "scheduled_audit_changes") || strings.Contains(string(r.Body), "CONFIDENTIAL EVIDENCE") {
		t.Fatalf("inbox summary %d %s", r.Status, r.Body)
	}
}

func TestAuditScheduleCreateIsIdempotent(t *testing.T) {
	c := controlled(t)
	in := scheduleInput(c)
	path := "/api/v1/audit-schedules"
	r := c.f.a.do("POST", path, in, c.f.analyst)
	if r.Status != 201 {
		t.Fatalf("create %d %s", r.Status, r.Body)
	}
	var first schedules.Schedule
	r.JSON(&first)
	r = c.f.a.do("POST", path, in, c.f.analyst)
	if r.Status != 201 {
		t.Fatalf("retry %d %s", r.Status, r.Body)
	}
	var again schedules.Schedule
	r.JSON(&again)
	if again.ID != first.ID {
		t.Fatal("duplicate schedule created")
	}
	in.Name = "Changed after ambiguous response"
	if r = c.f.a.do("POST", path, in, c.f.analyst); r.Status != 409 {
		t.Fatalf("changed request key %d %s", r.Status, r.Body)
	}
}

func TestAuditSchedulePreviewChecksWindowsAndTenantScope(t *testing.T) {
	c := controlled(t)
	in := scheduleInput(c)
	c.window(t, 0)
	r := c.f.a.do("POST", "/api/v1/audit-schedules/preview?org="+c.org.String(), in.Config.Rule, c.f.analyst)
	var preview struct {
		Items      []time.Time
		Restricted []bool
	}
	r.JSON(&preview)
	if r.Status != 200 || len(preview.Items) != 3 || len(preview.Restricted) != 3 || !preview.Restricted[0] {
		t.Fatalf("window preview %d %s", r.Status, r.Body)
	}
	if r = c.f.a.do("POST", "/api/v1/audit-schedules/preview?org="+uuid.NewString(), in.Config.Rule, c.f.analyst); r.Status != 404 {
		t.Fatalf("unknown org preview %d %s", r.Status, r.Body)
	}
}
