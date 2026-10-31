package server_test

import (
	"context"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/home"
	"testing"
)

func TestOrganizationHomeOverviewRespectsPermissionsAndTenant(t *testing.T) {
	f := newScanFixture(t, true)
	org := createOrg(t, f.a, f.admin, "home-org")
	r := f.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin)
	if r.Status != 204 {
		t.Fatalf("assign %d %s", r.Status, r.Body)
	}
	svc := &home.Service{DB: f.a.env.App}
	perms := []string{authz.OrganizationRead, authz.AssetRead, authz.FindingRead, authz.AssessmentRead, authz.ReportRead}
	p := &auth.Principal{TenantID: f.a.env.TenantA, Permissions: perms}
	summary, e := svc.Summary(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	if len(summary.Organizations) != 1 || summary.Organizations[0].Assets != 1 || summary.Organizations[0].Authorized != 1 || summary.Organizations[0].Risk != "No data yet" {
		t.Fatalf("unexpected overview %+v", summary.Organizations)
	}
	for _, omit := range perms {
		var subset []string
		for _, perm := range perms {
			if perm != omit {
				subset = append(subset, perm)
			}
		}
		p.Permissions = subset
		summary, e = svc.Summary(context.Background(), p)
		if e != nil {
			t.Fatal(e)
		}
		if len(summary.Organizations) != 0 {
			t.Fatalf("organization overview leaked without %s", omit)
		}
	}
	p.Permissions = perms
	p.TenantID = f.a.env.TenantB
	summary, e = svc.Summary(context.Background(), p)
	if e != nil || len(summary.Organizations) != 0 {
		t.Fatalf("cross-tenant overview %+v %v", summary, e)
	}
}
