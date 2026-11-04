package server_test

import (
	"orchis-go/internal/retention"
	"testing"
)

func TestRetentionHTTPPermissionsCASCSRFAndDeletion(t *testing.T) {
	a, e := setup(t)
	admin := a.login("admin@a.test")
	analyst := a.login("analyst@a.test")
	get := a.do("GET", "/api/v1/retention", nil, admin)
	if get.Status != 200 {
		t.Fatal(get.Status, string(get.Body))
	}
	var policy retention.Policy
	get.JSON(&policy)
	policy.EndpointDays = 14
	for _, method := range []string{"PUT", "GET"} {
		r := a.do(method, "/api/v1/retention", policy, analyst)
		if r.Status != 403 {
			t.Fatal("Analyst retention permission", r.Status)
		}
	}
	r := a.do("PUT", "/api/v1/retention", policy, admin, "X-Requested-With", "")
	if r.Status != 403 {
		t.Fatal("retention CSRF guard", r.Status)
	}
	r = a.do("PUT", "/api/v1/retention", policy, admin)
	if r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = a.do("PUT", "/api/v1/retention", policy, admin); r.Status != 409 {
		t.Fatal("stale policy version accepted", r.Status)
	}
	org := createOrg(t, a, admin, "delete-http")
	path := "/api/v1/organizations/" + org.ID.String()
	r = a.do("GET", path+"/deletion-preview", nil, admin)
	if r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	var preview retention.Preview
	r.JSON(&preview)
	input := retention.DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}
	if r = a.do("DELETE", path, input, admin, "X-Requested-With", ""); r.Status != 403 {
		t.Fatal("delete CSRF guard", r.Status)
	}
	if r = a.do("DELETE", path, input, analyst); r.Status != 403 {
		t.Fatal("Analyst deletion permitted", r.Status)
	}
	foreign := newApp(t, e, e.TenantB)
	foreign.user(e.TenantB, "admin@b.test", "admin")
	other := foreign.login("admin@b.test")
	if r = foreign.do("GET", path+"/deletion-preview", nil, other); r.Status != 404 {
		t.Fatal("foreign preview leaked", r.Status)
	}
	input.Counts.Assets++
	if r = a.do("DELETE", path, input, admin); r.Status != 409 {
		t.Fatal("stale summary accepted", r.Status)
	}
	input.Counts = preview.Counts
	r = a.do("DELETE", path, input, admin)
	if r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	var result retention.Deletion
	r.JSON(&result)
	if result.Status != "completed" || result.Pending != 0 {
		t.Fatal("empty organisation cleanup incomplete", result)
	}
	if r = a.do("DELETE", path, input, admin); r.Status != 404 {
		t.Fatal("deleted organisation resurrected", r.Status)
	}
	if r = foreign.do("GET", "/api/v1/data-deletions/"+result.ID.String(), nil, other); r.Status != 404 {
		t.Fatal("foreign deletion leaked", r.Status)
	}
}
