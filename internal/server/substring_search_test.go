package server_test

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/execution"
	"testing"
)

func TestSubstringSearchKeysetKeepsTenantAndDeletedAssetBoundaries(t *testing.T) {
	f := newScanFixture(t, true)
	ctx := context.Background()
	asset := uuid.MustParse(f.asset)
	deleted := uuid.New()
	if _, e := f.a.env.Owner.Exec(ctx, `INSERT INTO assets(id,tenant_id,name,category,deleted_at) VALUES($1,$2,'LOCAL SEARCH DELETED','host',now())`, deleted, f.a.env.TenantA); e != nil {
		t.Fatal(e)
	}
	expected := map[uuid.UUID]bool{}
	for i := 1; i <= 7; i++ {
		fid := uuid.New()
		tenant := f.a.env.TenantA
		var target any = asset
		if i == 6 {
			target = deleted
		}
		if i == 7 {
			tenant = f.a.env.TenantB
			target = nil
		}
		if i <= 5 {
			expected[fid] = true
		}
		if _, e := f.a.env.Owner.Exec(ctx, `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,last_seen_at) VALUES($1,$2,$3,'vulnerability',$4,$5,'LOCAL needle search','high','2026-01-01T00:00:00Z')`, fid, tenant, target, fmt.Sprintf("needle-%d", i), []byte(fmt.Sprintf("%032d", i))); e != nil {
			t.Fatal(e)
		}
	}
	svc := &execution.Service{DB: f.a.env.App}
	p := &auth.Principal{TenantID: f.a.env.TenantA, Permissions: []string{authz.FindingRead}}
	seen := map[uuid.UUID]bool{}
	cursor := ""
	for pageNumber := 0; pageNumber < 4; pageNumber++ {
		page, e := svc.ListFindings(ctx, p, execution.FindingFilter{Query: "needle", Severity: "high", Limit: 2, Cursor: cursor}, nil)
		if e != nil {
			t.Fatal(e)
		}
		if page.Search != "database" {
			t.Fatal("Fallback not exercised")
		}
		for _, item := range page.Items {
			if !expected[item.ID] || seen[item.ID] {
				t.Fatal("Leaked or repeated finding", item.ID)
			}
			seen[item.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("Expected five unique scoped results, got %d", len(seen))
	}
}
