package server_test

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"orchis-go/internal/execution"
	"testing"
)

func TestOrganizationFiltersScopePaginationSearchAndUntrustedIndexResults(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	viewer := a.login("viewer@a.test")
	oa := createOrg(t, a, admin, "filter-a")
	ob := createOrg(t, a, admin, "filter-b")
	makeAsset := func(org uuid.UUID, name string) string {
		t.Helper()
		r := a.do("POST", "/api/v1/assets", map[string]any{"category": "host", "name": name, "organization_id": org}, admin)
		if r.Status != 201 {
			t.Fatalf("create member: %d %s", r.Status, r.Body)
		}
		var out struct {
			ID             string
			OrganizationID *uuid.UUID `json:"organization_id"`
		}
		r.JSON(&out)
		if out.OrganizationID == nil || *out.OrganizationID != org {
			t.Fatal("asset was not assigned atomically")
		}
		return out.ID
	}
	aa, ab := makeAsset(oa.ID, "A"), makeAsset(ob.ID, "B")
	ids := []uuid.UUID{}
	for i := 0; i < 5; i++ {
		asset := aa
		if i >= 3 {
			asset = ab
		}
		id := uuid.New()
		ids = append(ids, id)
		if _, err := env.Owner.Exec(context.Background(), `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,last_seen_at) VALUES($1,$2,$3,'vulnerability',$4,$5,'scope needle','high','2026-01-01T00:00:00Z')`, id, env.TenantA, asset, fmt.Sprint(i), []byte(fmt.Sprintf("%032d", i))); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"org", "organization_id"} {
		r := a.do("GET", "/api/v1/assets?"+name+"="+oa.ID.String(), nil, viewer)
		var page struct{ Items []struct{ ID string } }
		r.JSON(&page)
		if r.Status != 200 || len(page.Items) != 1 || page.Items[0].ID != aa {
			t.Fatalf("asset scope: %d %s", r.Status, r.Body)
		}
		seen := map[uuid.UUID]bool{}
		cursor := ""
		for n := 0; n < 5; n++ {
			r = a.do("GET", "/api/v1/findings?"+name+"="+oa.ID.String()+"&q=needle&limit=1&cursor="+cursor, nil, viewer)
			var found execution.FindingPage
			r.JSON(&found)
			if r.Status != 200 {
				t.Fatalf("findings: %d %s", r.Status, r.Body)
			}
			for _, f := range found.Items {
				if f.AssetID == nil || f.AssetID.String() != aa || seen[f.ID] {
					t.Fatal("finding escaped organisation or repeated across cursor")
				}
				seen[f.ID] = true
			}
			if found.NextCursor == nil {
				break
			}
			cursor = *found.NextCursor
		}
		if len(seen) != 3 {
			t.Fatalf("scope pagination lost rows: %d", len(seen))
		}
	}
	svc := &execution.Service{DB: env.App}
	p := principalFor(t, env.TenantA)
	page, err := svc.ListFindings(context.Background(), p, execution.FindingFilter{OrganizationID: &oa.ID, Query: "needle"}, fixedSearch{ids: ids})
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("index results must be intersected with organisation: %+v %v", page, err)
	}
	page, err = svc.ListFindings(context.Background(), principalFor(t, env.TenantB), execution.FindingFilter{OrganizationID: &oa.ID, Query: "needle"}, fixedSearch{ids: ids})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("organisation filter leaked across tenant")
	}
	for _, path := range []string{"/assets", "/findings"} {
		if r := a.do("GET", "/api/v1"+path+"?org="+oa.ID.String()+"&organization_id="+ob.ID.String(), nil, viewer); r.Status != 422 {
			t.Fatalf("ambiguous org scope accepted: %d", r.Status)
		}
		if r := a.do("GET", "/api/v1"+path+"?org=not-a-uuid", nil, viewer); r.Status != 400 {
			t.Fatalf("invalid org accepted: %d", r.Status)
		}
	}
}

func TestAssetCreateOrganisationRequiresWriteAndSameTenantAndRejectsDemo(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	oa := createOrg(t, a, admin, "create-scope")
	if _, err := env.Owner.Exec(context.Background(), `UPDATE roles SET permissions=array_remove(permissions,'organization.write') WHERE tenant_id=$1 AND key='analyst'`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	analyst := a.login("analyst@a.test")
	body := map[string]any{"category": "host", "name": "member", "organization_id": oa.ID}
	if r := a.do("POST", "/api/v1/assets", body, analyst); r.Status != 403 {
		t.Fatalf("assignment without organisation.write: %d %s", r.Status, r.Body)
	}
	foreign := uuid.New()
	if _, err := env.Owner.Exec(context.Background(), `INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'Foreign','foreign')`, foreign, env.TenantB); err != nil {
		t.Fatal(err)
	}
	body["organization_id"] = foreign
	if r := a.do("POST", "/api/v1/assets", body, admin); r.Status != 404 {
		t.Fatalf("foreign org assignment: %d %s", r.Status, r.Body)
	}
	body["organization_id"] = oa.ID
	if _, err := env.Owner.Exec(context.Background(), `UPDATE organizations SET is_demo=true WHERE id=$1`, oa.ID); err != nil {
		t.Fatal(err)
	}
	if r := a.do("POST", "/api/v1/assets", body, admin); r.Status != 409 {
		t.Fatalf("demo org assignment: %d %s", r.Status, r.Body)
	}
	var n int
	if err := env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM assets WHERE name='member'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("refused assignment left an asset behind")
	}
}
