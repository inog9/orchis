package server_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http/httptest"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/scancontrol"
	"orchis-go/internal/scans"
	"orchis-go/internal/server"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type controlledScan struct {
	f       *scanFixture
	org     uuid.UUID
	input   scans.Input
	clock   *atomic.Int64
	options server.Options
}

func controlled(t *testing.T) *controlledScan {
	t.Helper()
	f := newScanFixture(t, true)
	clock := &atomic.Int64{}
	clock.Store(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).UnixNano())
	o := server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, Guard: contract.Guard{AllowLoopback: true}, ScanConcurrency: 1, Now: func() time.Time { return time.Unix(0, clock.Load()) }}
	f.a.srv.Close()
	f.a.srv = httptest.NewServer(server.New(o))
	t.Cleanup(f.a.srv.Close)
	org := createOrg(t, f.a, f.admin, "control")
	if r := f.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatalf("assign: %d %s", r.Status, r.Body)
	}
	return &controlledScan{f: f, org: org.ID, clock: clock, options: o, input: scans.Input{OrganizationID: org.ID, AssetIDs: []uuid.UUID{uuid.MustParse(f.asset)}, Tools: []string{"nmap", "nuclei"}, Preset: "quick"}}
}
func (c *controlledScan) path() string {
	return "/api/v1/organizations/" + c.org.String() + "/scan-control"
}
func (c *controlledScan) launch(t *testing.T, key string) scans.Scan {
	t.Helper()
	r := c.f.a.do("POST", "/api/v1/scans", c.input, c.f.analyst, "Idempotency-Key", key)
	if r.Status != 201 {
		t.Fatalf("launch: %d %s", r.Status, r.Body)
	}
	var out scans.Scan
	r.JSON(&out)
	return out
}
func (c *controlledScan) window(t *testing.T, version int) scancontrol.State {
	t.Helper()
	r := c.f.a.do("PUT", c.path(), scancontrol.Input{Version: version, Window: scancontrol.Window{Timezone: "UTC", Days: []int{1}, Start: "10:00", End: "11:00"}}, c.f.admin)
	if r.Status != 200 {
		t.Fatalf("window: %d %s", r.Status, r.Body)
	}
	var state scancontrol.State
	r.JSON(&state)
	return state
}
func (c *controlledScan) tick(t *testing.T) {
	t.Helper()
	if e := server.ScanService(c.options).Tick(context.Background(), c.options.Tenant); e != nil {
		t.Fatal(e)
	}
}
func (c *controlledScan) execution(t *testing.T, scan uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT e.id FROM executions e JOIN scan_jobs j ON j.simulation_id=e.simulation_id AND j.tenant_id=e.tenant_id WHERE j.assessment_id=$1 ORDER BY e.id LIMIT 1`, scan).Scan(&id); e != nil {
		t.Fatal(e)
	}
	return id
}
func (c *controlledScan) assertCanceled(t *testing.T, id uuid.UUID) {
	t.Helper()
	var state, outcome, reason string
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT state,COALESCE(outcome,''),COALESCE(error,'') FROM executions WHERE id=$1`, id).Scan(&state, &outcome, &reason); e != nil {
		t.Fatal(e)
	}
	if state != "ended" || outcome != "canceled" || reason == "" {
		t.Fatalf("not a documented stop: %s %s %q", state, outcome, reason)
	}
}

func TestScanWindowCreationCASAndExpiry(t *testing.T) {
	c := controlled(t)
	state := c.window(t, 0)
	if !state.AllowedNow {
		t.Fatal("open window reported closed")
	}
	if r := c.f.a.do("PUT", c.path(), scancontrol.Input{Version: 0}, c.f.admin); r.Status != 409 {
		t.Fatalf("stale window: %d", r.Status)
	}
	if r := c.f.a.do("PUT", c.path(), scancontrol.Input{Version: state.Version, Window: scancontrol.Window{Timezone: "Local"}}, c.f.admin); r.Status != 422 {
		t.Fatalf("invalid window: %d", r.Status)
	}
	c.clock.Store(time.Date(2026, 11, 2, 9, 59, 0, 0, time.UTC).UnixNano())
	if r := c.f.a.do("POST", "/api/v1/scans", c.input, c.f.analyst, "Idempotency-Key", "outside"); r.Status != 409 {
		t.Fatalf("creation outside window: %d %s", r.Status, r.Body)
	}
	if r := c.f.a.do("POST", "/api/v1/organizations/"+c.org.String()+"/assessments", map[string]any{"profile": "quick"}, c.f.analyst, "Idempotency-Key", "old-outside"); r.Status != 409 {
		t.Fatalf("legacy outside window: %d %s", r.Status, r.Body)
	}
	if r := c.f.a.do("POST", "/api/v1/atomic-tests/"+c.f.test+"/run", nil, c.f.analyst); r.Status != 409 {
		t.Fatalf("atomic outside window: %d %s", r.Status, r.Body)
	}
	c.clock.Store(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).UnixNano())
	scan := c.launch(t, "inside")
	c.tick(t)
	id := c.execution(t, scan.ID)
	// The claim guard must work independently of periodic workers.
	c.clock.Store(time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC).UnixNano())
	rt := server.Runtime(c.options)
	in, e := rt.Authenticate(context.Background(), c.f.token)
	if e != nil {
		t.Fatal(e)
	}
	// The first pending tool can be Nuclei; authenticate its corresponding integration.
	var key string
	if e = c.f.a.env.Owner.QueryRow(context.Background(), `SELECT i.key FROM executions e JOIN integrations i ON i.id=e.integration_id AND i.tenant_id=e.tenant_id WHERE e.id=$1`, id).Scan(&key); e != nil {
		t.Fatal(e)
	}
	if key != "nmap" {
		tok, e := execution.IssueToken(context.Background(), c.options.DB, c.options.Tenant, key)
		if e != nil {
			t.Fatal(e)
		}
		in, e = rt.Authenticate(context.Background(), tok)
		if e != nil {
			t.Fatal(e)
		}
	}
	if _, e = rt.Claim(context.Background(), in, id); e == nil {
		t.Fatal("claim crossed closed window")
	}
	c.assertCanceled(t, id)
	var audits int
	if e = c.f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE resource_id=$1 AND action='scan.policy_stop' AND detail->>'reason'='outside_window'`, id).Scan(&audits); e != nil || audits != 1 {
		t.Fatalf("claim policy audit missing: %d %v", audits, e)
	}
	c.tick(t)
	var waiting int
	if e = c.f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM scan_jobs WHERE assessment_id=$1 AND status IN ('waiting','running')`, scan.ID).Scan(&waiting); e != nil || waiting != 0 {
		t.Fatalf("queued jobs survived expiry: %d %v", waiting, e)
	}
}

func TestScanWindowHeartbeatAndLateResultAreCanceled(t *testing.T) {
	for _, mode := range []string{"heartbeat", "complete", "restart"} {
		t.Run(mode, func(t *testing.T) {
			c := controlled(t)
			c.window(t, 0)
			// Atomic work isolates each runtime gate from the assessment dispatcher.
			r := c.f.a.do("POST", "/api/v1/atomic-tests/"+c.f.test+"/run", nil, c.f.analyst)
			if r.Status != 202 {
				t.Fatalf("atomic queue: %d %s", r.Status, r.Body)
			}
			var queued struct {
				ID uuid.UUID `json:"id"`
			}
			r.JSON(&queued)
			rt := server.Runtime(c.options)
			in, e := rt.Authenticate(context.Background(), c.f.token)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = rt.Claim(context.Background(), in, queued.ID); e != nil {
				t.Fatal(e)
			}
			c.clock.Store(time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC).UnixNano())
			switch mode {
			case "heartbeat":
				cancel, e := rt.Heartbeat(context.Background(), in, queued.ID)
				if e != nil || !cancel {
					t.Fatalf("heartbeat did not stop: %v %v", cancel, e)
				}
			case "complete":
				zero := 0
				e := rt.Complete(context.Background(), in, queued.ID, execution.Result{ExitCode: &zero, Output: "late result"})
				if e == nil {
					t.Fatal("late successful result accepted outside window")
				}
			case "restart":
				fresh := server.Runtime(c.options)
				if _, _, e = fresh.Sweep(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			c.assertCanceled(t, queued.ID)
		})
	}
}

func TestEmergencyStopIsolationDurabilityCASAndAudit(t *testing.T) {
	c := controlled(t)
	a := c.launch(t, "a")
	c.tick(t)
	id := c.execution(t, a.ID)
	other := createOrg(t, c.f.a, c.f.admin, "other-control")
	asset := mkAsset(t, c.f.a, c.f.admin, map[string]any{"name": "other target", "category": "host", "organization_id": other.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.2"}}})
	in := scans.Input{OrganizationID: other.ID, AssetIDs: []uuid.UUID{uuid.MustParse(asset)}, Tools: []string{"nmap"}, Preset: "quick"}
	r := c.f.a.do("POST", "/api/v1/scans", in, c.f.analyst, "Idempotency-Key", "other")
	if r.Status != 201 {
		t.Fatal(r.Status)
	}
	var b scans.Scan
	r.JSON(&b)
	for _, cookie := range []string{c.f.analyst, c.f.viewer} {
		if r = c.f.a.do("POST", c.path()+"/stop", nil, cookie); r.Status != 403 {
			t.Fatalf("nonadmin stop: %d", r.Status)
		}
	}
	r = c.f.a.do("POST", c.path()+"/stop", nil, c.f.admin)
	if r.Status != 200 {
		t.Fatalf("stop: %d %s", r.Status, r.Body)
	}
	var hold scancontrol.State
	r.JSON(&hold)
	c.assertCanceled(t, id)
	if !hold.Paused || !strings.Contains(hold.PausedBy, "admin") {
		t.Fatalf("hold missing actor: %+v", hold)
	}
	if r = c.f.a.do("POST", c.path()+"/stop", nil, c.f.admin); r.Status != 200 {
		t.Fatal("stop replay")
	}
	var auditCount int
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action='scan_control.stop' AND resource_id=$1`, c.org).Scan(&auditCount); e != nil || auditCount != 1 {
		t.Fatalf("stop audit duplicate/missing: %d %v", auditCount, e)
	}
	if r = c.f.a.do("POST", "/api/v1/scans", c.input, c.f.analyst, "Idempotency-Key", "held"); r.Status != 409 {
		t.Fatalf("held launch: %d", r.Status)
	}
	c.tick(t)
	bid := c.execution(t, b.ID)
	var state string
	c.f.a.env.Owner.QueryRow(context.Background(), `SELECT state FROM executions WHERE id=$1`, bid).Scan(&state)
	if state != "ready" {
		t.Fatalf("other organisation stopped: %s", state)
	}
	// Fresh server sees durable organisation hold; global stop also stops the other organisation.
	c.f.a.srv.Close()
	c.f.a.srv = httptest.NewServer(server.New(c.options))
	t.Cleanup(c.f.a.srv.Close)
	r = c.f.a.do("POST", "/api/v1/scan-control/stop", nil, c.f.admin)
	if r.Status != 200 {
		t.Fatalf("global stop: %d %s", r.Status, r.Body)
	}
	var global scancontrol.State
	r.JSON(&global)
	c.assertCanceled(t, bid)
	if r = c.f.a.do("POST", c.path()+"/resume", scancontrol.Input{Version: hold.Version - 1}, c.f.admin); r.Status != 409 {
		t.Fatalf("stale resume: %d", r.Status)
	}
	if r = c.f.a.do("POST", c.path()+"/resume", scancontrol.Input{Version: hold.Version}, c.f.admin); r.Status != 200 {
		t.Fatal(r.Status)
	}
	if r = c.f.a.do("POST", "/api/v1/scans", c.input, c.f.analyst, "Idempotency-Key", "global-held"); r.Status != 409 {
		t.Fatalf("organisation resume bypassed global: %d", r.Status)
	}
	if r = c.f.a.do("POST", "/api/v1/scan-control/resume", scancontrol.Input{Version: global.Version}, c.f.admin); r.Status != 200 {
		t.Fatal(r.Status)
	}
	c.assertCanceled(t, id)
	c.assertCanceled(t, bid)
	// Cookie belongs to tenant B; both existence and writes must remain isolated.
	otherApp := newApp(t, c.f.a.env, c.f.a.env.TenantB)
	otherApp.user(c.f.a.env.TenantB, "control@b.test", "admin")
	cookie := otherApp.login("control@b.test")
	if r = otherApp.do("POST", c.path()+"/stop", nil, cookie); r.Status != 404 {
		t.Fatalf("cross-tenant stop: %d", r.Status)
	}
}

func TestEmergencyStopSerializesConcurrentLaunch(t *testing.T) {
	c := controlled(t)
	c.launch(t, "queued-before-race")
	var wg sync.WaitGroup
	start := make(chan struct{})
	statuses := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r := c.f.a.do("POST", "/api/v1/scans", c.input, c.f.analyst, "Idempotency-Key", fmt.Sprintf("race-%d", i))
			statuses <- r.Status
		}(i)
	}
	close(start)
	r := c.f.a.do("POST", c.path()+"/stop", nil, c.f.admin)
	if r.Status != 200 {
		t.Fatalf("stop: %d %s", r.Status, r.Body)
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 201 && status != 409 {
			t.Fatalf("racing launch: %d", status)
		}
	}
	var active int
	e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE a.organization_id=$1 AND j.status IN ('waiting','running')`, c.org).Scan(&active)
	if e != nil || active != 0 {
		t.Fatalf("job escaped committed stop: %d %v", active, e)
	}
}

func TestScanControlFrozenAndCurrentOrganisationCannotBeBypassed(t *testing.T) {
	for _, holdOriginal := range []bool{true, false} {
		t.Run(fmt.Sprint(holdOriginal), func(t *testing.T) {
			c := controlled(t)
			scan := c.launch(t, "frozen")
			c.tick(t)
			id := c.execution(t, scan.ID)
			other := createOrg(t, c.f.a, c.f.admin, "moved")
			// Inventory membership can change; the queued scan keeps its original organisation.
			if _, e := c.f.a.env.Owner.Exec(context.Background(), `UPDATE assets SET organization_id=$2 WHERE id=$1`, c.f.asset, other.ID); e != nil {
				t.Fatal(e)
			}
			org := other.ID
			if holdOriginal {
				org = c.org
			}
			r := c.f.a.do("POST", "/api/v1/organizations/"+org.String()+"/scan-control/stop", nil, c.f.admin)
			if r.Status != 200 {
				t.Fatalf("stop moved asset: %d %s", r.Status, r.Body)
			}
			c.assertCanceled(t, id)
			var frozen uuid.UUID
			if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT scan_organization_id FROM executions WHERE id=$1`, id).Scan(&frozen); e != nil || frozen != c.org {
				t.Fatalf("frozen organisation lost: %s %v", frozen, e)
			}
		})
	}
}

func TestScanControlChainCreationAndQueuedStepAreGuarded(t *testing.T) {
	c := newChainFixture(t)
	org := createOrg(t, c.a, c.admin, "chain-control")
	if r := c.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": c.asset, "version": 1}}}, c.admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	if r := c.targets(t, []string{c.asset}, nil); r.Status != 200 {
		t.Fatal(r.Status)
	}
	if r := c.graph(t, []map[string]any{step("scan", contract.KeyNmapTCP, "after_parents", map[string]any{"params": map[string]any{"profile": "top100"}})}, nil); r.Status != 200 {
		t.Fatalf("graph: %d %s", r.Status, r.Body)
	}
	path := "/api/v1/organizations/" + org.ID.String() + "/scan-control"
	if r := c.a.do("POST", path+"/stop", nil, c.admin); r.Status != 200 {
		t.Fatal(r.Status)
	}
	r := c.a.do("POST", "/api/v1/scenarios/"+c.scenario+"/launch", map[string]any{}, c.analyst)
	if r.Status != 409 {
		t.Fatalf("chain launch escaped hold: %d %s", r.Status, r.Body)
	}
}

func TestScanControlLockIsTransactionalAndTenantScoped(t *testing.T) {
	c := controlled(t)
	ctx := context.Background()
	first, e := c.f.a.env.Owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer first.Rollback(ctx)
	if e = scancontrol.Lock(ctx, first, c.options.Tenant); e != nil {
		t.Fatal(e)
	}
	second, e := c.f.a.env.Owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Rollback(ctx)
	if e = scancontrol.Lock(ctx, second, c.f.a.env.TenantB); e != nil {
		t.Fatal(e)
	}
	limited, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if e = scancontrol.Lock(limited, second, c.options.Tenant); e == nil {
		t.Fatal("scan mutation escaped tenant transaction lock")
	}
	if e = first.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	third, e := c.f.a.env.Owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer third.Rollback(ctx)
	if e = scancontrol.Lock(ctx, third, c.options.Tenant); e != nil {
		t.Fatal("transaction lock survived rollback")
	}
}

func TestScanControlServiceRequiresAdministrator(t *testing.T) {
	c := controlled(t)
	var user uuid.UUID
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT id FROM users WHERE tenant_id=$1 AND email='analyst@a.test'`, c.options.Tenant).Scan(&user); e != nil {
		t.Fatal(e)
	}
	p := &auth.Principal{TenantID: c.options.Tenant, UserID: user, Permissions: []string{authz.OrganizationRead}}
	_, e := server.ScanControlService(c.options).Change(context.Background(), p, &c.org, "stop", scancontrol.Input{}, "", "")
	var problem *httpx.Problem
	if !errors.As(e, &problem) || problem.Status != 403 {
		t.Fatalf("service allowed nonadministrator scan control: %v", e)
	}
}

func TestScanControlEngineStopsUnstartedChild(t *testing.T) {
	c := newChainFixture(t)
	org := createOrg(t, c.a, c.admin, "unstarted-child")
	if r := c.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": c.asset, "version": 1}}}, c.admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	if r := c.targets(t, []string{c.asset}, nil); r.Status != 200 {
		t.Fatal(r.Status)
	}
	if r := c.graph(t, []map[string]any{step("parent", contract.KeyNmapTCP, "after_parents", nil), step("child", contract.KeyNmapTCP, "after_parents", nil)}, [][2]string{{"parent", "child"}}); r.Status != 200 {
		t.Fatal(r.Status)
	}
	sim := c.launch(t)
	// Reproduce recovery after the parent committed but before a child was made.
	if _, e := c.a.env.Owner.Exec(context.Background(), `UPDATE executions SET state='ended',outcome='success',ended_at=now() WHERE simulation_id=$1`, sim.ID); e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/organizations/" + org.ID.String() + "/scan-control/stop"
	if r := c.a.do("POST", path, nil, c.admin); r.Status != 200 {
		t.Fatal(r.Status)
	}
	if _, _, e := server.SimulationService(server.Options{DB: c.a.env.App, Guard: contract.Guard{AllowLoopback: true}}).Tick(context.Background(), c.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	var state, outcome, reason string
	if e := c.a.env.Owner.QueryRow(context.Background(), `SELECT e.state,COALESCE(e.outcome,''),COALESCE(e.error,'') FROM executions e JOIN simulation_steps s ON s.tenant_id=e.tenant_id AND s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key='child'`, sim.ID).Scan(&state, &outcome, &reason); e != nil {
		t.Fatal(e)
	}
	if state != "ended" || outcome != "canceled" || !strings.Contains(reason, "emergency stop") {
		t.Fatalf("engine queued a child despite hold: %s %s %s", state, outcome, reason)
	}
	var status string
	if e := c.a.env.Owner.QueryRow(context.Background(), `SELECT status FROM simulations WHERE id=$1`, sim.ID).Scan(&status); e != nil {
		t.Fatal(e)
	}
	if status != "canceled" {
		t.Fatalf("policy stopped simulation not marked canceled: %s", status)
	}
}

func TestScanControlRLSAndCompositeOrganisationForeignKey(t *testing.T) {
	c := controlled(t)
	c.window(t, 0)
	ctx := context.Background()
	if e := c.options.DB.InTenantRead(ctx, c.f.a.env.TenantB, func(tx pgx.Tx) error {
		var count int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM scan_controls`).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			t.Fatalf("scan controls crossed tenant RLS: %d", count)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	for _, crossTenant := range []bool{true, false} {
		tenant := c.f.a.env.TenantB
		if crossTenant {
			tenant = c.options.Tenant
		}
		e := c.options.DB.InTenant(ctx, c.f.a.env.TenantB, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO scan_controls(id,tenant_id,organization_id) VALUES($1,$2,$3)`, uuid.New(), tenant, c.org)
			return e
		})
		var pg *pgconn.PgError
		code := "23503"
		if crossTenant {
			code = "42501"
		}
		if !errors.As(e, &pg) || pg.Code != code {
			t.Fatalf("cross-tenant control row accepted or wrong rejection: %v wanted %s", e, code)
		}
	}
}

func TestScanControlStoppedScanFinalizesWithAtomicSnapshot(t *testing.T) {
	c := controlled(t)
	scan := c.launch(t, "stop-snapshot")
	if r := c.f.a.do("POST", c.path()+"/stop", nil, c.f.admin); r.Status != 200 {
		t.Fatal(r.Status)
	}
	var unfrozen int
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM assessments a WHERE a.id=$1 AND a.status<>'running' AND NOT EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id)`, scan.ID).Scan(&unfrozen); e != nil || unfrozen != 0 {
		t.Fatalf("policy stop ended scan without atomic snapshot: %d %v", unfrozen, e)
	}
	c.tick(t)
	var status string
	var reports int
	if e := c.f.a.env.Owner.QueryRow(context.Background(), `SELECT a.status,(SELECT count(*) FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id AND r.snapshot->>'status'='canceled') FROM assessments a WHERE a.id=$1`, scan.ID).Scan(&status, &reports); e != nil {
		t.Fatal(e)
	}
	if status != "canceled" || reports != 1 {
		t.Fatalf("policy scan did not finalize canceled with one snapshot: %s %d", status, reports)
	}
	c.tick(t)
}
