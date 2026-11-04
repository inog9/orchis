package retention

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/authz"
	"orchis-go/internal/testutil"
	"testing"
)

func TestDeletionRequiresEveryRelatedPermission(t *testing.T) {
	e := testutil.New(t)
	p := admin(t, e, e.TenantA)
	s := Service{DB: e.App}
	for _, missing := range []string{authz.SettingsManage, authz.OrganizationWrite, authz.AssetDelete, authz.AgentManage, authz.FindingRead, authz.ReportRead} {
		t.Run(missing, func(t *testing.T) {
			other := *p
			other.Permissions = nil
			for _, perm := range p.Permissions {
				if perm != missing {
					other.Permissions = append(other.Permissions, perm)
				}
			}
			_, err := s.Preview(context.Background(), &other, uuid.New())
			problem(t, err, 403)
		})
	}
}
func TestArtifactQueuesRejectActiveRendererAndForeignRawKey(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a := fixture(t, e, p)
	s := Service{DB: e.App}
	if _, err := e.Owner.Exec(ctx, `UPDATE reports SET status='rendering' WHERE id=$1`, a.report); err != nil {
		t.Fatal(err)
	}
	err := e.App.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error { return queueReports(ctx, tx, p.TenantID, []uuid.UUID{a.report}, nil) })
	problem(t, err, 409)
	if _, err = e.Owner.Exec(ctx, `UPDATE reports SET status='ready' WHERE id=$1`, a.report); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Owner.Exec(ctx, `UPDATE executions SET raw_output_key='foreign/output.txt' WHERE id=$1`, a.execution); err != nil {
		t.Fatal(err)
	}
	preview, err := s.Preview(ctx, p, a.org)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}, "", "raw-key")
	problem(t, err, 409)
	var tasks int
	if err = e.Owner.QueryRow(ctx, `SELECT count(*) FROM data_cleanup_tasks`).Scan(&tasks); err != nil || tasks != 0 {
		t.Fatal("invalid artifact path left committed intents", err)
	}
}
