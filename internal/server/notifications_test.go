package server_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/alerts"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/osquery"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
	"orchis-go/internal/server"
	"strings"
	"testing"
	"time"
)

func notificationCount(t *testing.T, a *app, cookie, kind string) int {
	t.Helper()
	n := 0
	for _, x := range getAlerts(t, a, cookie, "").Items {
		if x.Kind == kind {
			n++
		}
	}
	return n
}
func TestNotificationsOfflinePolicyBoundariesEpisodesAndSafeFailures(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "notifications")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	svc := server.AgentService(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, Now: func() time.Time { return now }})
	var policy alerts.Policy
	r := f.a.do("GET", "/api/v1/alerts/policy", nil, f.admin)
	r.JSON(&policy)
	if r.Status != 200 || policy.OfflineMinutes != 15 || policy.Version != 0 {
		t.Fatal("safe notification defaults", r.Status, string(r.Body))
	}
	if r = f.a.do("PUT", "/api/v1/alerts/policy", alerts.Policy{OfflineMinutes: 2}, f.analyst); r.Status != 403 {
		t.Fatal("analyst changed notification policy", r.Status)
	}
	if r = f.a.do("PUT", "/api/v1/alerts/policy", alerts.Policy{OfflineMinutes: 2}, f.admin, "X-Requested-With", ""); r.Status != 403 {
		t.Fatal("notification CSRF", r.Status)
	}
	for _, minutes := range []int{1, 10081} {
		if r = f.a.do("PUT", "/api/v1/alerts/policy", alerts.Policy{OfflineMinutes: minutes}, f.admin); r.Status != 422 {
			t.Fatal("offline bounds", r.Status)
		}
	}
	setBeat := func(age time.Duration) {
		t.Helper()
		if _, e := f.a.env.Owner.Exec(ctx, `UPDATE agents SET last_heartbeat_at=$2 WHERE id=$1`, ag.AgentID, now.Add(-age)); e != nil {
			t.Fatal(e)
		}
	}
	sweep := func() {
		t.Helper()
		if _, e := svc.Sweep(ctx); e != nil {
			t.Fatal(e)
		}
	}
	setBeat(2 * time.Minute)
	sweep()
	if n := notificationCount(t, f.a, f.analyst, "agent_offline"); n != 0 {
		t.Fatal("90-second indicator must not trigger 15-minute notification", n)
	}
	setBeat(15 * time.Minute)
	sweep()
	if n := notificationCount(t, f.a, f.analyst, "agent_offline"); n != 0 {
		t.Fatal("threshold is strictly exceeded", n)
	}
	now = now.Add(time.Second)
	sweep()
	sweep()
	if n := notificationCount(t, f.a, f.analyst, "agent_offline"); n != 1 {
		t.Fatal("one notification per offline episode", n)
	}
	heartbeat := func(code string) {
		t.Helper()
		r := f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1, "diagnostic_code": code}, ag.Secret)
		if r.Status != 200 {
			t.Fatal("heartbeat", r.Status, string(r.Body))
		}
	}
	heartbeat("update_failed")
	heartbeat("release_key_missing")
	heartbeat("release_key_missing")
	if n := notificationCount(t, f.a, f.analyst, "agent_update_failed"); n != 1 {
		t.Fatal("update diagnostic episode must dedupe", n)
	}
	heartbeat("")
	heartbeat("update_failed")
	if n := notificationCount(t, f.a, f.analyst, "agent_update_failed"); n != 2 {
		t.Fatal("new failure after recovery must notify", n)
	}
	if r = f.a.do("PUT", "/api/v1/alerts/policy", alerts.Policy{OfflineMinutes: 2}, f.admin); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = f.a.do("PUT", "/api/v1/alerts/policy", alerts.Policy{OfflineMinutes: 3}, f.admin); r.Status != 409 {
		t.Fatal("policy CAS bypass", r.Status)
	}
	setBeat(3 * time.Minute)
	sweep()
	sweep()
	if n := notificationCount(t, f.a, f.analyst, "agent_offline"); n != 2 {
		t.Fatal("configured threshold and new episode", n)
	}
	heartbeat("SECRET-not-a-diagnostic")
	if r = f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.admin); r.Status != 204 {
		t.Fatal("revoke", r.Status, string(r.Body))
	}
	if n := notificationCount(t, f.a, f.analyst, "agent_revoked"); n != 1 {
		t.Fatal("revocation event missing", n)
	}
	r = f.a.do("GET", "/api/v1/alerts", nil, f.analyst)
	if strings.Contains(string(r.Body), "SECRET-not-a-diagnostic") {
		t.Fatal("raw diagnostic leaked")
	}
	if n := len(getAlerts(t, f.a, f.a.login("viewer@a.test"), "").Items); n != 0 {
		t.Fatal("Viewer must not see agent notifications", n)
	}
}
func TestNotificationsOrganizationTenantAndCurrentObjectVisibility(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	o1 := createOrg(t, a, admin, "notifications-one")
	o2 := createOrg(t, a, admin, "notifications-two")
	asset1 := mkAsset(t, a, admin, map[string]any{"name": "one", "category": "host", "organization_id": o1.ID})
	asset2 := mkAsset(t, a, admin, map[string]any{"name": "two", "category": "host", "organization_id": o2.ID})
	ctx := context.Background()
	svc := alerts.Service{DB: env.App}
	var user uuid.UUID
	if e := env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND email='admin@a.test'`, env.TenantA).Scan(&user); e != nil {
		t.Fatal(e)
	}
	p := &auth.Principal{TenantID: env.TenantA, UserID: user, Permissions: authz.SystemRoles[0].Permissions}
	if e := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
		for _, raw := range []string{asset1, asset2} {
			id := uuid.MustParse(raw)
			if e := alerts.Raise(ctx, tx, env.TenantA, alerts.New{Kind: "authorization_pending", Title: "Review authorization", ResourceType: "asset", ResourceID: &id, Audience: authz.AssetAuthorize, DedupeKey: raw}); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	list, e := svc.ListOrganization(ctx, p, false, 100, &o1.ID)
	if e != nil || len(list.Items) != 1 || list.Unread != 1 || list.Items[0].ResourceLabel == nil || *list.Items[0].ResourceLabel != "one" || list.Items[0].OrganizationID == nil || *list.Items[0].OrganizationID != o1.ID {
		t.Fatal("organization filter mismatch", list, e)
	}
	one := list.Items[0].ID
	other, e := svc.ListOrganization(ctx, p, false, 100, &o2.ID)
	if e != nil || len(other.Items) != 1 || other.Unread != 1 {
		t.Fatal("second organization isolation", other, e)
	}
	if e = svc.Read(ctx, p, one); e != nil {
		t.Fatal(e)
	}
	other, e = svc.ListOrganization(ctx, p, false, 100, &o2.ID)
	if e != nil || other.Unread != 1 {
		t.Fatal("read leaked to other organization", other, e)
	}
	narrow := *p
	narrow.Permissions = []string{authz.AlertRead, authz.AssetAuthorize}
	if list, e = svc.List(ctx, &narrow, false, 100); e != nil || len(list.Items) != 0 || list.Unread != 0 {
		t.Fatal("related object permissions bypass", list, e)
	}
	if e = svc.Read(ctx, &narrow, other.Items[0].ID); e == nil {
		t.Fatal("mark must enforce object visibility")
	}
	if e = svc.ReadAll(ctx, &narrow); e != nil {
		t.Fatal(e)
	}
	other, e = svc.ListOrganization(ctx, p, false, 100, &o2.ID)
	if e != nil || other.Unread != 1 {
		t.Fatal("hidden notification marked by read-all", other, e)
	}
	foreign := *p
	foreign.TenantID = env.TenantB
	if list, e = svc.List(ctx, &foreign, false, 100); e != nil || len(list.Items) != 0 || list.Unread != 0 {
		t.Fatal("notification tenant leak", list, e)
	}
	if e = svc.Read(ctx, &foreign, one); e == nil {
		t.Fatal("cross-tenant read accepted")
	}
	finding := uuid.New()
	if _, e = env.Owner.Exec(ctx, `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title) VALUES($1,$2,$3,'open_port','tcp/8080',$4,'Fixture finding')`, finding, env.TenantA, asset1, make([]byte, 32)); e != nil {
		t.Fatal(e)
	}
	if e = env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
		return alerts.Raise(ctx, tx, env.TenantA, alerts.New{Kind: "new_critical_finding", Title: "Finding notification", ResourceType: "finding", ResourceID: &finding, Audience: authz.FindingRead})
	}); e != nil {
		t.Fatal(e)
	}
	viewer := *p
	viewer.Permissions = []string{authz.FindingRead, authz.ReportRead}
	found, e := svc.ListOrganization(ctx, &viewer, false, 100, &o1.ID)
	if e != nil || len(found.Items) != 1 {
		t.Fatal("finding reader notification missing", found, e)
	}
	findingAlert := found.Items[0].ID
	if _, e = env.Owner.Exec(ctx, `UPDATE assets SET organization_id=$2 WHERE id=$1`, asset1, o2.ID); e != nil {
		t.Fatal(e)
	}
	if e = svc.Read(ctx, &viewer, findingAlert); e == nil {
		t.Fatal("moved finding organization association leaked")
	}

	if e = svc.Read(ctx, p, one); e == nil {
		t.Fatal("stale organization association must fail closed")
	}
	if list, e = svc.ListOrganization(ctx, p, false, 100, &o1.ID); e != nil || len(list.Items) != 0 {
		t.Fatal("old org notification remained visible", list, e)
	}
}
func TestNotificationsScanTerminalAndReportFailureAreAtomicAndIdempotent(t *testing.T) {
	f := newScanFixture(t, true)
	o := createOrg(t, f.a, f.admin, "notification-scan")
	ctx := context.Background()
	if r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatal(r.Status, string(r.Body))
	}
	web := mkAsset(t, f.a, f.admin, map[string]any{"category": "web_application", "name": "Notification web", "organization_id": o.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.1:8080/"}}})
	r := f.a.do("POST", "/api/v1/scans", scans.Input{OrganizationID: o.ID, AssetIDs: []uuid.UUID{uuid.MustParse(web)}}, f.analyst, "Idempotency-Key", "notify-stop")
	var scan scans.Scan
	r.JSON(&scan)
	if r.Status != 201 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = f.a.do("POST", "/api/v1/scans/"+scan.ID.String()+"/cancel", nil, f.analyst); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	scanSvc := server.ScanService(server.Options{DB: f.a.env.App, Guard: contract.Guard{AllowLoopback: true}})
	if e := scanSvc.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if n := notificationCount(t, f.a, f.analyst, "scan_stopped"); n != 1 {
		t.Fatal("scan terminal event must be exactly once", n)
	}
	reports := reporting.Service{DB: f.a.env.App}
	if e := reports.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if e := reports.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if n := notificationCount(t, f.a, f.analyst, "pdf_failed"); n != 1 {
		t.Fatal("failed PDF attempt must notify exactly once", n)
	}
	viewer := f.a.login("viewer@a.test")
	if n := notificationCount(t, f.a, viewer, "pdf_failed"); n != 1 {
		t.Fatal("Viewer report notification missing", n)
	}
	if n := notificationCount(t, f.a, viewer, "scan_stopped"); n != 0 {
		t.Fatal("Viewer scan details leaked", n)
	}
	// An explicit retry begins a new rendering generation even though attempts reset.
	var retryReport uuid.UUID
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM reports WHERE assessment_id=$1`, scan.ID).Scan(&retryReport); e != nil {
		t.Fatal(e)
	}
	if r = f.a.do("POST", "/api/v1/reports/"+retryReport.String()+"/retry", nil, f.analyst); r.Status != 202 {
		t.Fatal("retry generation", r.Status, string(r.Body))
	}
	if e := reports.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if n := notificationCount(t, f.a, viewer, "pdf_failed"); n != 2 {
		t.Fatal("new PDF retry failure must notify", n)
	}
	var report uuid.UUID
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM reports WHERE assessment_id=$1`, scan.ID).Scan(&report); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE reports SET status='rendering',attempts=3,lease_until=now()-interval '1 minute' WHERE id=$1`, report); e != nil {
		t.Fatal(e)
	}
	if e := reports.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if n := notificationCount(t, f.a, viewer, "pdf_failed"); n != 3 {
		t.Fatal("expired worker lease failure notification missing", n)
	}
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("%PDF-local-renderer-protocol-fixture")) }))
	defer renderer.Close()
	reports.Store = &endpointStoreSpy{}
	reports.RendererURL = renderer.URL
	reports.RendererToken = strings.Repeat("x", 32)
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE reports SET status='queued',attempts=0 WHERE id=$1`, report); e != nil {
		t.Fatal(e)
	}
	if e := reports.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if e := reports.Tick(ctx, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	if n := notificationCount(t, f.a, viewer, "report_ready"); n != 1 {
		t.Fatal("ready notification not visible to frozen report reader", n)
	}
	var actor uuid.UUID
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND email='admin@a.test'`, f.a.env.TenantA).Scan(&actor); e != nil {
		t.Fatal(e)
	}
	limited := &auth.Principal{TenantID: f.a.env.TenantA, UserID: actor, Permissions: []string{authz.ReportRead}}
	inbox := alerts.Service{DB: f.a.env.App}
	if list, e := inbox.List(ctx, limited, false, 100); e != nil || len(list.Items) != 0 {
		t.Fatal("report notification requires finding read", list, e)
	}

}

func TestNotificationsPendingApprovalsAndPolicyRLS(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "pending-review")
	o := createOrg(t, f.a, f.admin, "pending-review")
	ctx := context.Background()
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE agents SET organization_id=$2 WHERE id=$1`, ag.AgentID, o.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE assets SET organization_id=$2 WHERE id=(SELECT asset_id FROM agents WHERE id=$1)`, ag.AgentID, o.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `INSERT INTO osquery_org_settings(tenant_id,organization_id,query_key,enabled) VALUES($1,$2,'osquery.inventory',true)`, f.a.env.TenantA, o.ID); e != nil {
		t.Fatal(e)
	}
	svc := server.AgentService(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA})
	for i := 0; i < 2; i++ {
		if _, e := svc.Sweep(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for _, kind := range []string{"authorization_pending", "consent_pending"} {
		if n := notificationCount(t, f.a, f.admin, kind); n != 1 {
			t.Fatal("admin pending event must dedupe", kind, n)
		}
		if n := notificationCount(t, f.a, f.analyst, kind); n != 0 {
			t.Fatal("pending admin approval leaked to analyst", kind, n)
		}
	}
	var actor uuid.UUID
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND email='admin@a.test'`, f.a.env.TenantA).Scan(&actor); e != nil {
		t.Fatal(e)
	}
	p := &auth.Principal{TenantID: f.a.env.TenantA, UserID: actor, Permissions: authz.SystemRoles[0].Permissions}
	policySvc := alerts.Service{DB: f.a.env.App}
	narrow := *p
	narrow.Permissions = []string{authz.AlertRead}
	if _, e := policySvc.GetPolicy(ctx, &narrow); e == nil {
		t.Fatal("policy read direct permission bypass")
	}
	if _, e := policySvc.SetPolicy(ctx, &narrow, alerts.Policy{OfflineMinutes: 5}, "", ""); e == nil {
		t.Fatal("policy write direct permission bypass")
	}
	if _, e := policySvc.SetPolicy(ctx, p, alerts.Policy{OfflineMinutes: 5}, "", ""); e != nil {
		t.Fatal(e)
	}
	if e := f.a.env.App.InTenantRead(ctx, f.a.env.TenantB, func(tx pgx.Tx) error {
		var n int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM notification_policies`).Scan(&n); e != nil {
			return e
		}
		if n != 0 {
			t.Fatal("notification policy tenant RLS missing", n)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	foreign := *p
	foreign.TenantID = f.a.env.TenantB
	other, e := policySvc.GetPolicy(ctx, &foreign)
	if e != nil || other.OfflineMinutes != 15 || other.Version != 0 {
		t.Fatal("cross tenant policy leak", other, e)
	}
	// Current consent prevents new renewal alerts. No missing choice is silently enabled.
	c := osquery.Consent()
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE osquery_org_settings SET consent_version=$2,consent_sha256=$3,consented_by=$4,consented_at=now() WHERE organization_id=$1`, o.ID, c.Version, c.SHA256, actor); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Sweep(ctx); e != nil {
		t.Fatal(e)
	}
	if n := notificationCount(t, f.a, f.admin, "consent_pending"); n != 1 {
		t.Fatal("current consent caused a second pending alert", n)
	}
}

func TestNotificationsDeletedObjectsAndRelatedPermissionsFailClosed(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "visible-agent")
	ctx := context.Background()
	var user uuid.UUID
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND email='admin@a.test'`, f.a.env.TenantA).Scan(&user); e != nil {
		t.Fatal(e)
	}
	p := &auth.Principal{TenantID: f.a.env.TenantA, UserID: user, Permissions: authz.SystemRoles[0].Permissions}
	svc := alerts.Service{DB: f.a.env.App}
	ghosts := []uuid.UUID{}
	if e := f.a.env.App.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		id := uuid.MustParse(ag.AgentID)
		if e := alerts.Raise(ctx, tx, p.TenantID, alerts.New{Kind: "agent_update_failed", Title: "Known agent", ResourceType: "agent", ResourceID: &id, Audience: authz.AgentRead}); e != nil {
			return e
		}
		for _, kind := range []string{"agent", "asset", "finding", "report", "scan", "simulation", "execution", "user"} {
			alertID, missing := uuid.New(), uuid.New()
			ghosts = append(ghosts, alertID)
			if _, e := tx.Exec(ctx, `INSERT INTO alerts(id,tenant_id,kind,title,resource_type,resource_id,audience_permission) VALUES($1,$2,'deleted-object','Deleted private object',$3,$4,'alert.read')`, alertID, p.TenantID, kind, missing); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	list, e := svc.List(ctx, p, false, 100)
	if e != nil || len(list.Items) != 1 || list.Unread != 1 {
		t.Fatal("deleted object notification exposed", list, e)
	}
	for _, id := range ghosts {
		if e = svc.Read(ctx, p, id); e == nil {
			t.Fatal("deleted object marked visible", id)
		}
	}
	narrow := *p
	narrow.Permissions = []string{authz.AlertRead, authz.AgentRead}
	if list, e = svc.List(ctx, &narrow, false, 100); e != nil || len(list.Items) != 0 || list.Unread != 0 {
		t.Fatal("agent alert missing asset read permitted", list, e)
	}
	o := createOrg(t, f.a, f.admin, "visible-agent-org")
	if _, e = f.a.env.Owner.Exec(ctx, `UPDATE agents SET organization_id=$2 WHERE id=$1`, ag.AgentID, o.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.a.env.Owner.Exec(ctx, `UPDATE assets SET organization_id=$2 WHERE id=(SELECT asset_id FROM agents WHERE id=$1)`, ag.AgentID, o.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.a.env.App.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		id := uuid.MustParse(ag.AgentID)
		return alerts.Raise(ctx, tx, p.TenantID, alerts.New{Kind: "agent_update_failed", Title: "Assigned agent", ResourceType: "agent", ResourceID: &id, Audience: authz.AgentRead})
	}); e != nil {
		t.Fatal(e)
	}
	narrow.Permissions = []string{authz.AlertRead, authz.AgentRead, authz.AssetRead}
	if list, e = svc.List(ctx, &narrow, false, 100); e != nil || len(list.Items) != 0 {
		t.Fatal("assigned agent requires organization permission", list, e)
	}
	noRead := *p
	noRead.Permissions = nil
	if _, e = svc.List(ctx, &noRead, false, 100); e == nil {
		t.Fatal("inbox without a read grant allowed")
	}
}
