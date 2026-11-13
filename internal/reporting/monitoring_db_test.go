package reporting

import (
	"context"
	"strings"
	"testing"

	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
)

func TestMonitoringFindingsFrozenIntoReportAndRendered(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	org, asset := ids.New(), ids.New()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := env.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations (id, tenant_id, name, slug) VALUES ($1,$2,'O','o')`, org, env.TenantA)
	must(`INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'host','h','manual')`, asset, env.TenantA, org)
	must(`INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details,organization_id) VALUES ($1,$2,$3,'c2_suspect_connection','203.0.113.9','h1','Suspicious outbound connections to 203.0.113.9','high','{"dst":"203.0.113.9","connections":12,"ports":[8443],"processes":["python.exe"],"secret_payload":"must-not-leak"}',$4)`, ids.New(), env.TenantA, asset, org)
	must(`INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,organization_id) VALUES ($1,$2,$3,'vulnerability','CVE-1','h2','not monitoring','high',$4)`, ids.New(), env.TenantA, asset, org)
	tx, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	got, err := monitoringFindings(ctx, tx, env.TenantA, asset)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != "c2_suspect_connection" || len(got[0].MITRE) == 0 {
		t.Fatalf("only monitor detections belong in the monitoring chapter: %+v", got)
	}
	if _, leaked := got[0].Evidence["secret_payload"]; leaked || got[0].Evidence["dst"] != "203.0.113.9" || got[0].Evidence["processes"] != "python.exe" {
		t.Fatalf("evidence must be whitelisted: %+v", got[0].Evidence)
	}
	for _, lang := range []string{"en", "id"} {
		snap := Snapshot{Profile: "standard", Language: lang, Assets: []Asset{{ID: asset, Name: "host-1", Status: "finished", Monitoring: got}}}
		out, err := HTML(snap)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "203.0.113.9") || !strings.Contains(string(out), `id="monitoring"`) || strings.Contains(string(out), "must-not-leak") {
			t.Fatalf("%s report must render the monitoring chapter", lang)
		}
	}
}
