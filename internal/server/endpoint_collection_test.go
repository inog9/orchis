package server_test

import (
	"context"
	"orchis-go/internal/endpoint"
	"orchis-go/internal/osquery"
	"orchis-go/internal/server"
	"testing"
	"time"
)

func TestEndpointCollectionRequiresExplicitConsentReusesDefinitionsAndRespectsDueTime(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "periodic")
	org := f.joinOrg(t, asset)
	ctx := context.Background()
	svc := server.EndpointService(server.Options{DB: f.a.env.App})
	count := func(query string) int {
		t.Helper()
		var n int
		if e := f.a.env.Owner.QueryRow(ctx, query, asset).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	if n := count(`SELECT count(*) FROM executions WHERE target_asset_id=$1`); n != 0 {
		t.Fatalf("automatic collection before consent: %d", n)
	}
	// Manual baseline juga tidak dapat memakai fallback rendah tanpa dokumen saat ini.
	id := f.createRun(t, secret, asset, "osquery.inventory")
	if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 204 {
		t.Fatalf("baseline without consent reached agent: %d %s", r.Status, r.Body)
	}
	if got := f.exec(t, id); got.Outcome != "success" || !logHas(got.Logs, osquery.ReasonDisabled) {
		t.Fatalf("missing explicit skip: %+v", got)
	}
	in := endpoint.CollectionInput{Platform: "linux", Versions: map[string]int64{}, Consent: osquery.Consent().ConsentRef, Keys: []string{}}
	for _, q := range osquery.Queries {
		if q.SupportsOS("linux") {
			in.Keys = append(in.Keys, q.Key)
			in.Versions[q.Key] = 0
		}
	}
	path := "/api/v1/organizations/" + org + "/endpoint-collection"
	if r := f.a.do("PUT", path, in, f.analyst); r.Status != 403 {
		t.Fatalf("collection approval without settings.manage: %d", r.Status)
	}
	bad := in
	bad.Consent.Version = "old"
	if r := f.a.do("PUT", path, bad, f.admin); r.Status != 422 {
		t.Fatalf("old automatic consent accepted: %d", r.Status)
	}
	if r := f.a.do("PUT", path, in, f.admin); r.Status != 204 {
		t.Fatalf("bulk consent %d %s", r.Status, r.Body)
	}
	if r := f.a.do("PUT", path, in, f.admin); r.Status != 409 {
		t.Fatalf("stale collection CAS accepted: %d", r.Status)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	nkeys := len(in.Keys)
	scheduled := `SELECT count(*) FROM endpoint_collection_schedule d JOIN agents g ON g.tenant_id=d.tenant_id AND g.id=d.agent_id WHERE g.asset_id=$1`
	if n := count(scheduled); n != nkeys {
		t.Fatalf("supported queries not queued after consent: %d want %d", n, nkeys)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	auto := `SELECT count(*) FROM executions e WHERE e.target_asset_id=$1 AND e.atomic_test_id IN(SELECT atomic_test_id FROM endpoint_collection_schedule)`
	if n := count(auto); n != nkeys {
		t.Fatalf("periodic duplicate while busy: %d", n)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE endpoint_collection_schedule SET last_enqueued_at=now()-interval '16 minutes' WHERE agent_id IN(SELECT id FROM agents WHERE asset_id=$1)`, asset); e != nil {
		t.Fatal(e)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	if n := count(auto); n != nkeys {
		t.Fatalf("busy query was collected twice: %d", n)
	}
	// Fixture menyelesaikan antrean secara sintetis agar clock jatuh tempo dapat diuji tanpa tidur15menit.
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE endpoint_collection_schedule SET last_enqueued_at=now() WHERE agent_id IN(SELECT id FROM agents WHERE asset_id=$1)`, asset); e != nil {
		t.Fatal(e)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE executions SET state='ended',outcome='success',ended_at=now() WHERE target_asset_id=$1`, asset); e != nil {
		t.Fatal(e)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	if n := count(auto); n != nkeys {
		t.Fatalf("collection ran before due time: %d", n)
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE endpoint_collection_schedule SET last_enqueued_at=now()-interval '16 minutes' WHERE agent_id IN(SELECT id FROM agents WHERE asset_id=$1)`, asset); e != nil {
		t.Fatal(e)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	if n := count(auto); n != 2*nkeys {
		t.Fatalf("periodic due was not collected: %d", n)
	}
	if n := count(scheduled); n != nkeys {
		t.Fatalf("scheduler grew definitions every interval: %d", n)
	}
	// Pencabutan consent sesudah enqueue memblokir seluruh query, termasuk baseline rendah.
	if _, e := f.a.env.Owner.Exec(ctx, `DELETE FROM osquery_org_settings WHERE organization_id=$1`, org); e != nil {
		t.Fatal(e)
	}
	if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 204 {
		t.Fatalf("queued collection after consent deletion reached agent: %d %s", r.Status, r.Body)
	}
}

func TestEndpointCollectionRejectsUnsupportedPlatformsAndUnknownQueries(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "platform")
	org := f.joinOrg(t, asset)
	path := "/api/v1/organizations/" + org + "/endpoint-collection"
	for _, platform := range []string{"windows", "ios", "linux;sh"} {
		in := endpoint.CollectionInput{Platform: platform, Consent: osquery.Consent().ConsentRef}
		if r := f.a.do("PUT", path, in, f.admin); r.Status != 422 {
			t.Fatalf("unsupported automatic platform %s: %d", platform, r.Status)
		}
	}
	in := endpoint.CollectionInput{Platform: "linux", Keys: []string{"SELECT * FROM users"}, Consent: osquery.Consent().ConsentRef}
	if r := f.a.do("PUT", path, in, f.admin); r.Status != 422 {
		t.Fatalf("free query accepted: %d", r.Status)
	}
}

func TestEndpointCollectionSchedulerNeverUsesUnapprovedRevokedOrUnsupportedAgents(t *testing.T) {
	f, org, asset, enrolled := networkSourceFixture(t)
	ctx := context.Background()
	svc := server.EndpointService(server.Options{DB: f.a.env.App})
	tests := []struct {
		name, change, restore string
		arg                   any
	}{
		{"consent", `UPDATE osquery_org_settings SET consent_version='old' WHERE organization_id=$1`, `UPDATE osquery_org_settings SET consent_version='` + osquery.ConsentVersion + `' WHERE organization_id=$1`, org},
		{"disabled", `UPDATE osquery_org_settings SET enabled=false WHERE organization_id=$1`, `UPDATE osquery_org_settings SET enabled=true WHERE organization_id=$1`, org},
		{"revoked", `UPDATE agents SET status='revoked' WHERE id=$1`, `UPDATE agents SET status='online' WHERE id=$1`, enrolled.AgentID},
		{"demo", `UPDATE agents SET is_demo=true WHERE id=$1`, `UPDATE agents SET is_demo=false WHERE id=$1`, enrolled.AgentID},
		{"platform", `UPDATE agents SET os='windows' WHERE id=$1`, `UPDATE agents SET os='linux' WHERE id=$1`, enrolled.AgentID},
		{"architecture", `UPDATE agents SET arch='arm64' WHERE id=$1`, `UPDATE agents SET arch='x86_64' WHERE id=$1`, enrolled.AgentID},
		{"testing", `UPDATE assets SET testing_authorized=false WHERE id=$1`, `UPDATE assets SET testing_authorized=true WHERE id=$1`, asset},
		{"organisation", `UPDATE assets SET organization_id=NULL WHERE id=$1`, `UPDATE assets SET organization_id='` + org.String() + `' WHERE id=$1`, asset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, e := f.a.env.Owner.Exec(ctx, tt.change, tt.arg); e != nil {
				t.Fatal(e)
			}
			err := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute)
			if _, e := f.a.env.Owner.Exec(ctx, tt.restore, tt.arg); e != nil {
				t.Fatal(e)
			}
			if err != nil {
				t.Fatalf("eligible-only collection failed (%s): %v", tt.name, err)
			}
			var n int
			f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM endpoint_collection_schedule WHERE tenant_id=$1`, f.a.env.TenantA).Scan(&n)
			if n != 0 {
				t.Fatalf("unsafe automatic agent accepted (%s): %d", tt.name, n)
			}
		})
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantB, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	var n int
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM endpoint_collection_schedule WHERE tenant_id=$1`, f.a.env.TenantA).Scan(&n)
	if n != 1 {
		t.Fatalf("eligible automatic agent missing: %d", n)
	}
}

func TestOsqueryUnpinnedPlatformsNeverReceiveNamedQueries(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "unsupported-osquery", "osquery.inventory")
	ctx := context.Background()
	for _, p := range []struct{ os, arch string }{{"windows", "x86_64"}, {"linux", "arm64"}} {
		if _, e := f.a.env.Owner.Exec(ctx, `UPDATE agents SET os=$2,arch=$3 WHERE asset_id=$1`, asset, p.os, p.arch); e != nil {
			t.Fatal(e)
		}
		f.createRun(t, secret, asset, "osquery.inventory")
		if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 204 {
			t.Fatalf("unpinned platform received osquery (%s/%s): %d %s", p.os, p.arch, r.Status, r.Body)
		}
		status, reason := f.datasetStatus(t, asset, "osquery.inventory")
		if status != "skipped" || reason != osquery.ReasonUnsupported {
			t.Fatalf("unsupported platform was not explicit: %s %s", status, reason)
		}
	}
}

// macOS (Intel dan Apple Silicon) punya artefak yang dipatok: query yang mendukung macOS sampai ke agent.
func TestOsqueryPinnedMacOSReceivesNamedQueries(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "macos-osquery", "osquery.inventory")
	ctx := context.Background()
	for _, arch := range []string{"x86_64", "arm64"} {
		if _, e := f.a.env.Owner.Exec(ctx, `UPDATE agents SET os='macos',arch=$2 WHERE asset_id=$1`, asset, arch); e != nil {
			t.Fatal(e)
		}
		f.createRun(t, secret, asset, "osquery.inventory")
		if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 200 {
			t.Fatalf("pinned macOS %s did not receive osquery.inventory: %d %s", arch, r.Status, r.Body)
		}
	}
}

// Persetujuan dari tampilan macOS hanya menyentuh query yang mendukung macOS; query khusus Linux tidak ikut dinonaktifkan.
func TestEndpointCollectionMacOSOnlyTouchesMacQueries(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "mac-collection")
	org := f.joinOrg(t, asset)
	path := "/api/v1/organizations/" + org + "/endpoint-collection"
	versions := map[string]int64{}
	for _, q := range osquery.Queries {
		if q.SupportsOS("macos") {
			versions[q.Key] = 0
		}
	}
	in := endpoint.CollectionInput{Platform: "macos", Keys: []string{"osquery.inventory"}, Versions: versions, Consent: osquery.Consent().ConsentRef}
	if r := f.a.do("PUT", path, in, f.admin); r.Status >= 300 {
		t.Fatalf("macOS collection approval: %d %s", r.Status, r.Body)
	}
	ctx := context.Background()
	var enabled, linuxOnly int
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND enabled`, org).Scan(&enabled); e != nil {
		t.Fatal(e)
	}
	if enabled != 1 {
		t.Fatalf("exactly the chosen macOS query must be enabled: %d", enabled)
	}
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND query_key IN('osquery.packages','osquery.firewall')`, org).Scan(&linuxOnly); e != nil {
		t.Fatal(e)
	}
	if linuxOnly != 0 {
		t.Fatalf("a macOS approval wrote settings for Linux-only queries: %d", linuxOnly)
	}
	// Linux-only query diminta dari tampilan macOS ditolak.
	bad := endpoint.CollectionInput{Platform: "macos", Keys: []string{"osquery.firewall"}, Versions: versions, Consent: osquery.Consent().ConsentRef}
	if r := f.a.do("PUT", path, bad, f.admin); r.Status != 422 {
		t.Fatalf("Linux-only query accepted for macOS: %d", r.Status)
	}
}

// Penjadwal otomatis juga mengumpulkan dari agent macOS yang online dan disetujui.
func TestEndpointCollectionSchedulerIncludesMacOSAgents(t *testing.T) {
	f, _, _, enrolled := networkSourceFixture(t)
	ctx := context.Background()
	svc := server.EndpointService(server.Options{DB: f.a.env.App})
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE agents SET os='macos' WHERE id=$1`, enrolled.AgentID); e != nil {
		t.Fatal(e)
	}
	if e := svc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal(e)
	}
	var n int
	f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM endpoint_collection_schedule WHERE tenant_id=$1`, f.a.env.TenantA).Scan(&n)
	if n != 1 {
		t.Fatalf("macOS agent was not scheduled: %d", n)
	}
}
