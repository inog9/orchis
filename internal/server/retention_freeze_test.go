package server_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/server"
	"testing"
)

func TestExpiredResultsAndReportsAreNeverFrozenAgain(t *testing.T) {
	a, e := setup(t)
	ctx := context.Background()
	admin := a.login("admin@a.test")
	org := createOrg(t, a, admin, "expired-freeze")
	for _, scan := range []bool{false, true} {
		for _, column := range []string{"results_expired_at", "report_expired_at"} {
			id := uuid.New()
			if _, err := e.Owner.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by,status,is_scan,`+column+`) SELECT $1,$2,$3,'Expired fixture','quick',$4,'fixture',id,'finished',$5,now() FROM users WHERE tenant_id=$2 AND email='admin@a.test'`, id, e.TenantA, org.ID, id.String(), scan); err != nil {
				t.Fatal(err)
			}
		}
	}
	opts := server.Options{DB: e.App, Tenant: e.TenantA}
	if err := server.ScanService(opts).Tick(ctx, e.TenantA); err != nil {
		t.Fatal(err)
	}
	if err := e.App.InTenant(ctx, e.TenantA, func(tx pgx.Tx) error { return server.ReportService(opts).Freeze(ctx, tx, e.TenantA) }); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM reports WHERE tenant_id=$1`, e.TenantA).Scan(&n); err != nil || n != 0 {
		t.Fatalf("expired snapshot recreated: %d %v", n, err)
	}
}
