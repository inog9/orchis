package retention

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/testutil"
	"testing"
)

func TestOrganizationDeletionPreservesEveryOtherOrganisationRecord(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a, b := fixture(t, e, p), fixture(t, e, p)
	s := Service{DB: e.App}
	v, err := s.Preview(ctx, p, a.org)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: v.Name, Version: v.Version, Counts: v.Counts}, "", "scope"); err != nil {
		t.Fatal(err)
	}
	for table, id := range map[string]uuid.UUID{"findings": b.finding, "reports": b.report, "executions": b.execution, "assets": b.asset, "agents": b.agent, "assessments": b.assessment} {
		var n int
		if err = e.Owner.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id=$1`, id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("other organisation record changed: %s %v", table, err)
		}
	}
	var state string
	if err = e.Owner.QueryRow(ctx, `SELECT status FROM agents WHERE id=$1`, b.agent).Scan(&state); err != nil || state != "online" {
		t.Fatal("other organisation agent revoked", err)
	}
}
