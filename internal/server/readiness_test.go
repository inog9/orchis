package server_test

import (
	"orchis-go/internal/organizations"
	"strings"
	"testing"
)

func TestOrganizationReadinessMatchesLaunchScopeAndTenant(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	viewer := a.login("viewer@a.test")
	o := createOrg(t, a, admin, "readiness")
	specs := []struct {
		name       string
		authorized bool
		addresses  []map[string]any
	}{
		{"unapproved", false, []map[string]any{{"kind": "ip", "value": "127.0.0.1"}}},
		{"no-address", true, nil},
		{"metadata", true, []map[string]any{{"kind": "ip", "value": "169.254.169.254"}}},
		{"ready", true, []map[string]any{{"kind": "ip", "value": "127.0.0.1"}, {"kind": "url", "value": "http://127.0.0.1:8080"}}},
	}
	for _, spec := range specs {
		aid := mkAsset(t, a, admin, map[string]any{"category": "host", "name": spec.name, "testing_authorized": spec.authorized, "addresses": spec.addresses})
		r := a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": aid, "version": 1}}}, admin)
		if r.Status != 204 {
			t.Fatalf("assign: %d %s", r.Status, r.Body)
		}
	}
	r := a.do("GET", "/api/v1/organizations/"+o.ID.String()+"/readiness", nil, viewer)
	var v organizations.Readiness
	r.JSON(&v)
	if r.Status != 200 || v.Total != 4 || v.Authorized != 3 || v.Ready != 1 {
		t.Fatalf("readiness incorrect: %d %s", r.Status, r.Body)
	}
	for _, item := range v.Items {
		switch item.Name {
		case "ready":
			if item.Reason != "" {
				t.Fatal(item.Reason)
			}
		case "unapproved":
			if item.Reason != "Not authorized for testing" {
				t.Fatal(item.Reason)
			}
		default:
			if !strings.HasPrefix(item.Reason, "Target refused:") {
				t.Fatal(item)
			}
		}
	}
	other := newApp(t, env, env.TenantB)
	other.user(env.TenantB, "readiness@b.test", "admin")
	cookie := other.login("readiness@b.test")
	if r := other.do("GET", "/api/v1/organizations/"+o.ID.String()+"/readiness", nil, cookie); r.Status != 404 {
		t.Fatalf("cross tenant %d", r.Status)
	}
	// Pratinjau tidak membuat assessment maupun eksekusi.
	var list struct{ Items []any }
	r = a.do("GET", "/api/v1/organizations/"+o.ID.String()+"/assessments", nil, admin)
	r.JSON(&list)
	if len(list.Items) != 0 {
		t.Fatal("preview launched assessment")
	}
}
