package server_test

import (
	"context"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/execution"
	"orchis-go/internal/reporting"
	"orchis-go/internal/server"
	"strings"
	"testing"
)

func TestReportSnapshotIsFrozenRedactedAndTenantScoped(t *testing.T) {
	f := newScanFixture(t, true)
	if _, e := f.a.env.Owner.Exec(context.Background(), `INSERT INTO mitre_techniques(id,name) VALUES('T1046','Network Service Discovery')`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.env.Owner.Exec(context.Background(), `INSERT INTO contract_techniques(tenant_id,contract_id,technique_id) SELECT tenant_id,id,'T1046' FROM contracts WHERE tenant_id=$1 AND key='nmap.tcp_connect'`, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	o := createOrg(t, f.a, f.admin, "report")
	r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin)
	if r.Status != 204 {
		t.Fatalf("assign %d %s", r.Status, r.Body)
	}
	r = f.a.do("POST", "/api/v1/organizations/"+o.ID.String()+"/assessments", map[string]any{}, f.analyst, "Idempotency-Key", "report")
	if r.Status != 202 {
		t.Fatalf("launch %d %s", r.Status, r.Body)
	}
	var a struct{ ID string }
	r.JSON(&a)
	token, e := execution.IssueToken(context.Background(), f.a.env.App, f.a.env.TenantA, "nuclei")
	if e != nil {
		t.Fatal(e)
	}
	rows, e := f.a.env.Owner.Query(context.Background(), `SELECT e.id,c.key FROM executions e JOIN contracts c ON c.id=e.contract_id AND c.tenant_id=e.tenant_id JOIN assessment_targets t ON t.simulation_id=e.simulation_id WHERE t.assessment_id=$1`, a.ID)
	if e != nil {
		t.Fatal(e)
	}
	type task struct{ id, key string }
	tasks := []task{}
	for rows.Next() {
		var x task
		rows.Scan(&x.id, &x.key)
		tasks = append(tasks, x)
	}
	rows.Close()
	for _, x := range tasks {
		tok := f.token
		findings := []map[string]any{}
		if x.key == "nuclei.scan" {
			tok = token
			findings = append(findings, map[string]any{"type": "vulnerability", "value": "exposure@test", "title": "Finding password=hunter2", "severity": "high", "details": map[string]any{"response": "DATABASE_PASSWORD=SHOULD-NOT-EXPORT", "template_id": "demo", "matched_at": "http://127.0.0.1/?token=SECRET"}})
		}
		if r = f.inj("POST", "executions/"+x.id+"/claim", nil, tok); r.Status != 200 {
			t.Fatalf("claim %d %s", r.Status, r.Body)
		}
		if r = f.inj("POST", "executions/"+x.id+"/result", map[string]any{"exit_code": 0, "findings": findings}, tok); r.Status != 204 {
			t.Fatalf("complete %d %s", r.Status, r.Body)
		}
	}
	svc := server.OrganizationService(server.Options{DB: f.a.env.App})
	reports := &reporting.Service{DB: f.a.env.App}
	if e = f.a.env.App.InTenant(context.Background(), f.a.env.TenantA, func(tx pgx.Tx) error {
		if e := svc.Refresh(context.Background(), tx, f.a.env.TenantA); e != nil {
			return e
		}
		return reports.Freeze(context.Background(), tx, f.a.env.TenantA)
	}); e != nil {
		t.Fatal(e)
	}
	r = f.a.do("GET", "/api/v1/reports?assessment_id="+a.ID, nil, f.viewer)
	if r.Status != 200 {
		t.Fatalf("reports %d %s", r.Status, r.Body)
	}
	var page struct{ Items []reporting.Report }
	r.JSON(&page)
	if len(page.Items) != 1 {
		t.Fatalf("expected one report %s", r.Body)
	}
	id := page.Items[0].ID.String()
	original := f.a.do("GET", "/api/v1/reports/"+id+"/export/json", nil, f.viewer)
	if original.Status != 200 {
		t.Fatalf("json %d %s", original.Status, original.Body)
	}
	if !strings.Contains(string(original.Body), "T1046") {
		t.Fatal("missing frozen MITRE mapping")
	}
	if compact := strings.ReplaceAll(string(original.Body), " ", ""); !strings.Contains(compact, `"basis":"estimate"`) || !strings.Contains(compact, `"id":"T1190"`) {
		t.Fatal("vulnerability finding must carry a frozen ATT&CK estimate (T1190)")
	}

	for _, format := range []string{"html", "csv"} {
		if removed := f.a.do("GET", "/api/v1/reports/"+id+"/export/"+format, nil, f.viewer); removed.Status != 404 {
			t.Fatalf("removed report format %s: %d", format, removed.Status)
		}
	}
	var full reporting.Report
	f.a.do("GET", "/api/v1/reports/"+id, nil, f.viewer).JSON(&full)
	printData, e := reporting.HTML(full.Snapshot)
	if e != nil || !strings.Contains(string(printData), "T1046") {
		t.Fatal("internal print report omitted frozen mapping")
	}
	for _, secret := range []string{"hunter2", "SHOULD-NOT-EXPORT", "?token=SECRET"} {
		if strings.Contains(string(original.Body), secret) {
			t.Fatalf("report leaked %s", secret)
		}
	}
	f.a.env.Owner.Exec(context.Background(), `UPDATE finding_occurrences SET finding_snapshot=jsonb_set(finding_snapshot,'{title}','"Changed evidence later"') WHERE execution_id IN(SELECT id FROM executions WHERE target_asset_id=$1)`, f.asset)
	f.a.env.Owner.Exec(context.Background(), `UPDATE findings SET title='Changed later',severity='critical' WHERE asset_id=$1`, f.asset)
	later := f.a.do("GET", "/api/v1/reports/"+id+"/export/json", nil, f.viewer)
	if string(later.Body) != string(original.Body) {
		t.Fatal("historical report changed")
	}
	f.a.env.App.InTenant(context.Background(), f.a.env.TenantA, func(tx pgx.Tx) error { return reports.Freeze(context.Background(), tx, f.a.env.TenantA) })
	afterRefreeze := f.a.do("GET", "/api/v1/reports/"+id+"/export/json", nil, f.viewer)
	if string(afterRefreeze.Body) != string(original.Body) {
		t.Fatal("snapshot changed when worker repeated")
	}
	var n int
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM reports WHERE assessment_id=$1`, a.ID).Scan(&n)
	if n != 1 {
		t.Fatal("snapshot generated twice")
	}
	other := newApp(t, f.a.env, f.a.env.TenantB)
	other.user(f.a.env.TenantB, "reports@b.test", "admin")
	cookie := other.login("reports@b.test")
	if r = other.do("GET", "/api/v1/reports/"+id, nil, cookie); r.Status != 404 {
		t.Fatalf("cross tenant report %d", r.Status)
	}
	// MinIO tidak dikonfigurasi dalam fixture ini: worker harus gagal jelas, snapshot tetap tersedia dan dapat dicoba ulang.
	f.a.env.Owner.Exec(context.Background(), `UPDATE reports SET status='rendering',attempts=1,lease_until=now()-interval '1 minute' WHERE id=$1`, id)
	if e = reports.Tick(context.Background(), f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	var attempts int
	var state string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT attempts,status FROM reports WHERE id=$1`, id).Scan(&attempts, &state)
	if attempts != 2 || state != "failed" {
		t.Fatalf("stale lease not recovered: %d %s", attempts, state)
	}
	r = f.a.do("POST", "/api/v1/reports/"+id+"/retry", nil, f.analyst)
	if r.Status != 202 {
		t.Fatalf("retry %d %s", r.Status, r.Body)
	}
	if r = f.a.do("POST", "/api/v1/reports/"+id+"/retry", nil, f.viewer); r.Status != 403 {
		t.Fatalf("viewer retry %d", r.Status)
	}
}
