package server_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/organizations"
	"strings"
	"testing"
)

func createOrg(t *testing.T, a *app, cookie, slug string) organizations.Organization {
	t.Helper()
	r := a.do("POST", "/api/v1/organizations", map[string]any{"name": "Example Corporation", "slug": slug}, cookie)
	if r.Status != 201 {
		t.Fatalf("organization create: %d %s", r.Status, r.Body)
	}
	var o organizations.Organization
	r.JSON(&o)
	return o
}
func TestOrganizationPermissionsVersionTenantAndMembership(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	analyst := a.login("analyst@a.test")
	viewer := a.login("viewer@a.test")
	if r := a.do("POST", "/api/v1/organizations", map[string]any{"name": "Bad", "slug": "bad"}, viewer); r.Status != 403 {
		t.Fatalf("viewer write %d", r.Status)
	}
	o := createOrg(t, a, admin, "example")
	if r := a.do("POST", "/api/v1/organizations", map[string]any{"name": "Duplicate", "slug": "example"}, admin); r.Status != 409 {
		t.Fatalf("duplicate slug: %d %s", r.Status, r.Body)
	}
	body := map[string]any{"name": "Updated", "slug": "example", "version": o.Version}
	if r := a.do("PATCH", "/api/v1/organizations/"+o.ID.String(), body, analyst); r.Status != 200 {
		t.Fatalf("patch %d %s", r.Status, r.Body)
	}
	if r := a.do("PATCH", "/api/v1/organizations/"+o.ID.String(), body, admin); r.Status != 409 {
		t.Fatalf("stale version %d", r.Status)
	}
	other := newApp(t, env, env.TenantB)
	other.user(env.TenantB, "org-admin@b.test", "admin")
	badmin := other.login("org-admin@b.test")
	if r := other.do("GET", "/api/v1/organizations/"+o.ID.String(), nil, badmin); r.Status != 404 {
		t.Fatalf("cross tenant %d", r.Status)
	}
	aid := mkAsset(t, a, admin, map[string]any{"category": "host", "name": "member"})
	member := map[string]any{"assets": []map[string]any{{"id": aid, "version": 1}}, "testing_authorized": true}
	if r := a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", member, analyst); r.Status != 403 {
		t.Fatalf("analyst authorize %d", r.Status)
	}
	if r := a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", member, admin); r.Status != 204 {
		t.Fatalf("membership %d %s", r.Status, r.Body)
	}
	if r := a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", member, admin); r.Status != 409 {
		t.Fatalf("stale asset %d", r.Status)
	}
	var page struct {
		Items []struct {
			ID             string
			OrganizationID *uuid.UUID `json:"organization_id"`
			Authorized     bool       `json:"testing_authorized"`
		}
	}
	r := a.do("GET", "/api/v1/assets?organization_id="+o.ID.String(), nil, analyst)
	r.JSON(&page)
	if r.Status != 200 || len(page.Items) != 1 || page.Items[0].OrganizationID == nil || !page.Items[0].Authorized {
		t.Fatalf("asset filter %d %s", r.Status, r.Body)
	}
	if e := env.App.InTenantRead(context.Background(), env.TenantB, func(tx pgx.Tx) error {
		var count int
		if e := tx.QueryRow(context.Background(), `SELECT count(*) FROM organizations WHERE id=$1`, o.ID).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			t.Fatal("RLS exposed organization")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
func TestOrganizationEnrollmentInstancesAndSingleUse(t *testing.T) {
	f := newAgentFixture(t)
	o := createOrg(t, f.a, f.admin, "agents")
	ids := map[string]bool{}
	for _, name := range []string{"alpha", "beta"} {
		r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "user", "organization_id": o.ID}, f.admin)
		if r.Status != 201 {
			t.Fatalf("token %d %s", r.Status, r.Body)
		}
		var tk struct{ Token string }
		r.JSON(&tk)
		body := f.enrollBody(tk.Token, "same-host")
		body["name"] = name
		r = f.agentCall("POST", "enroll", body, "")
		if r.Status != 200 {
			t.Fatalf("enroll %d %s", r.Status, r.Body)
		}
		var enrolled enrollOut
		r.JSON(&enrolled)
		if ids[enrolled.AgentID] {
			t.Fatal("instance IDs collide")
		}
		ids[enrolled.AgentID] = true
		if r = f.agentCall("POST", "enroll", body, ""); r.Status != 400 {
			t.Fatalf("token replay %d", r.Status)
		}
	}
	r := f.a.do("GET", "/api/v1/assets?organization_id="+o.ID.String(), nil, f.admin)
	var page struct {
		Items []struct {
			ID         string
			Authorized bool `json:"testing_authorized"`
		}
	}
	r.JSON(&page)
	if len(page.Items) != 2 {
		t.Fatalf("two assets %s", r.Body)
	}
	for _, a := range page.Items {
		if a.Authorized {
			t.Fatal("enrollment granted scan authorization")
		}
	}
	r = f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "user", "organization_id": o.ID}, f.admin)
	var tk struct{ Token string }
	r.JSON(&tk)
	body := f.enrollBody(tk.Token, "another-host")
	body["name"] = "alpha"
	if r = f.agentCall("POST", "enroll", body, ""); r.Status != 409 {
		t.Fatalf("duplicate instance %d %s", r.Status, r.Body)
	}
	body["name"] = "gamma"
	if r = f.agentCall("POST", "enroll", body, ""); r.Status != 200 {
		t.Fatalf("failed enrollment must roll token back %d %s", r.Status, r.Body)
	}
}
func TestAssessmentScopeIdempotencyFrozenTargetsAndCancel(t *testing.T) {
	f := newScanFixture(t, true)
	o := createOrg(t, f.a, f.admin, "assessment")
	skipped := mkAsset(t, f.a, f.admin, map[string]any{"category": "host", "name": "unauthorized", "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.1"}}})
	metadata := mkAsset(t, f.a, f.admin, map[string]any{"category": "host", "name": "metadata", "testing_authorized": true, "addresses": []map[string]any{{"kind": "ip", "value": "169.254.169.254"}}})
	members := map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}, {"id": skipped, "version": 1}, {"id": metadata, "version": 1}}}
	if r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", members, f.admin); r.Status != 204 {
		t.Fatalf("members %d %s", r.Status, r.Body)
	}
	path := "/api/v1/organizations/" + o.ID.String() + "/assessments"
	r := f.a.do("POST", path, map[string]any{"profile": "quick"}, f.analyst, "Idempotency-Key", "one")
	if r.Status != 202 {
		t.Fatalf("assessment %d %s", r.Status, r.Body)
	}
	var a organizations.Assessment
	r.JSON(&a)
	if len(a.Targets) != 3 {
		t.Fatalf("targets %s", r.Body)
	}
	var sim uuid.UUID
	reasons := 0
	for _, target := range a.Targets {
		if target.SimulationID != nil {
			sim = *target.SimulationID
		} else {
			reasons++
		}
	}
	if reasons != 2 || sim == uuid.Nil {
		t.Fatalf("partial skip %s", r.Body)
	}
	r = f.a.do("POST", path, map[string]any{"profile": "quick"}, f.analyst, "Idempotency-Key", "one")
	var again organizations.Assessment
	r.JSON(&again)
	if r.Status != 202 || again.ID != a.ID {
		t.Fatalf("idempotency %d %s", r.Status, r.Body)
	}
	if r = f.a.do("POST", path, map[string]any{"profile": "web"}, f.analyst, "Idempotency-Key", "one"); r.Status != 409 {
		t.Fatalf("different body %d", r.Status)
	}
	if r = f.a.do("POST", path, map[string]any{"asset_ids": []string{uuid.NewString()}}, f.analyst, "Idempotency-Key", "outside"); r.Status != 422 {
		t.Fatalf("outside scope %d %s", r.Status, r.Body)
	}
	var count int
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE simulation_id=$1`, sim).Scan(&count)
	if count != 2 {
		t.Fatalf("expected two executions, got %d", count)
	}
	// Perubahan alamat aset tidak boleh mengganti snapshot assessment.
	f.a.env.Owner.Exec(context.Background(), `UPDATE asset_addresses SET value='127.0.0.2' WHERE asset_id=$1`, f.asset)
	r = f.a.do("GET", "/api/v1/assessments/"+a.ID.String(), nil, f.analyst)
	if !strings.Contains(string(r.Body), "127.0.0.1") || strings.Contains(string(r.Body), "127.0.0.2") {
		t.Fatalf("snapshot changed %s", r.Body)
	}
	var executionID string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT e.id FROM executions e JOIN contracts c ON c.id=e.contract_id AND c.tenant_id=e.tenant_id WHERE e.simulation_id=$1 AND c.key='nmap.tcp_connect'`, sim).Scan(&executionID)
	if r = f.a.do("PATCH", "/api/v1/assets/"+f.asset, map[string]any{"version": 2, "testing_authorized": false}, f.admin); r.Status != 200 {
		t.Fatalf("revoke %d %s", r.Status, r.Body)
	}
	if r = f.inj("POST", "executions/"+executionID+"/claim", nil, f.token); r.Status != 409 {
		t.Fatalf("assessment claim ignored revoked authorization %d %s", r.Status, r.Body)
	}
	if r = f.a.do("POST", "/api/v1/assessments/"+a.ID.String()+"/cancel", nil, f.analyst); r.Status != 200 {
		t.Fatalf("cancel %d %s", r.Status, r.Body)
	}
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE simulation_id=$1 AND state<>'ended'`, sim).Scan(&count)
	if count != 0 {
		t.Fatal("cancel left active executions")
	}
}
