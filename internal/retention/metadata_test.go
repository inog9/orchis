package retention

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/testutil"
	"testing"
)

func TestOrganizationDeletionRemovesRelatedAlertAndOutboxContentsOnly(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a, b := fixture(t, e, p), fixture(t, e, p)
	s := Service{DB: e.App}
	for _, f := range []orgFixture{a, b} {
		for kind, id := range map[string]uuid.UUID{"organization": f.org, "asset": f.asset, "agent": f.agent, "finding": f.finding, "report": f.report, "assessment": f.assessment, "execution": f.execution} {
			if _, err := e.Owner.Exec(ctx, `INSERT INTO alerts(id,tenant_id,kind,title,resource_type,resource_id) VALUES($1,$2,'fixture','PRIVATE FIXTURE',$3,$4)`, uuid.New(), p.TenantID, kind, id); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Owner.Exec(ctx, `INSERT INTO outbox(message_id,tenant_id,topic,aggregate_type,aggregate_id,payload) VALUES($1,$2,'fixture',$3,$4,'{"private":"fixture"}')`, uuid.New(), p.TenantID, kind, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	v, err := s.Preview(ctx, p, a.org)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: v.Name, Version: v.Version, Counts: v.Counts}, "", "metadata"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"alerts", "outbox"} {
		var n int
		if err = e.Owner.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, p.TenantID).Scan(&n); err != nil || n != 7 {
			t.Fatalf("wrong scoped related metadata cleanup %s: %d %v", table, n, err)
		}
	}
}
