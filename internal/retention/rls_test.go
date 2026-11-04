package retention

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/testutil"
	"testing"
)

func TestRetentionTablesForceTenantIsolation(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	for _, tenant := range []uuid.UUID{e.TenantA, e.TenantB} {
		op := uuid.New()
		for _, v := range []struct {
			q    string
			args []any
		}{
			{`INSERT INTO retention_policies(tenant_id) VALUES($1)`, []any{tenant}},
			{`INSERT INTO data_deletions(id,tenant_id,organization_id,organization_name,counts) VALUES($1,$2,$3,'Fixture','{}')`, []any{op, tenant, uuid.New()}},
			{`INSERT INTO data_cleanup_tasks(tenant_id,kind,object_id,operation_id) VALUES($1,'report_pdf',$2,$3)`, []any{tenant, uuid.New(), op}},
		} {
			if _, err := e.Owner.Exec(ctx, v.q, v.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, table := range []string{"retention_policies", "data_deletions", "data_cleanup_tasks"} {
		t.Run(table, func(t *testing.T) {
			for _, tenant := range []uuid.UUID{e.TenantA, e.TenantB} {
				var n int
				err := e.App.InTenantRead(ctx, tenant, func(tx pgx.Tx) error { return tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n) })
				if err != nil || n != 1 {
					t.Fatalf("tenant RLS %s: %d %v", table, n, err)
				}
			}
			var n int
			if err := e.App.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
				t.Fatalf("missing context must deny %s: %d %v", table, n, err)
			}
		})
	}
}
