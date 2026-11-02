package reporting

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
	"testing"
	"time"
)

func TestFreezeScanSnapshotCannotOverwriteAnExistingSnapshot(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	org, scan, actor := ids.New(), ids.New(), ids.New()
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := env.Owner.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'Frozen scan','frozen-scan')`, org, env.TenantA)
	must(`INSERT INTO users(id,tenant_id,email,display_name,role_id) SELECT $1,$2,'freeze@a.test','Fixture',id FROM roles WHERE tenant_id=$2 AND key='admin'`, actor, env.TenantA)
	must(`INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by,is_scan,status) VALUES($1,$2,$3,'Frozen scan','quick','fixture','fixture',$4,true,'finished')`, scan, env.TenantA, org, actor)
	svc := Service{DB: env.App}
	snapshot := Snapshot{SchemaVersion: "orchis.report.v1", ScanID: &scan, AssessmentID: scan, Organization: "Original organisation", Profile: "quick", CreatedAt: time.Now().UTC(), Status: "finished", Assets: []Asset{}, BySeverity: map[string]int{}}
	freeze := func() {
		t.Helper()
		if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error { return svc.FreezeScanSnapshot(ctx, tx, env.TenantA, org, scan, snapshot) }); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []byte {
		t.Helper()
		var raw []byte
		if err := env.Owner.QueryRow(ctx, `SELECT snapshot FROM reports WHERE tenant_id=$1 AND assessment_id=$2`, env.TenantA, scan).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	freeze()
	before := read()
	snapshot.Organization = "Changed organisation"
	snapshot.BySeverity = map[string]int{"critical": 99}
	freeze()
	if !bytes.Equal(before, read()) {
		t.Fatal("existing scan snapshot was overwritten")
	}
	var n int
	if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM reports WHERE tenant_id=$1 AND assessment_id=$2`, env.TenantA, scan).Scan(&n); err != nil || n != 1 {
		t.Fatalf("snapshot count %d %v", n, err)
	}
	// Tenant lain tidak dapat membaca record walau mengetahui UUID root.
	if err := env.App.InTenantRead(ctx, env.TenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM reports WHERE assessment_id=$1`, uuid.UUID(scan)).Scan(&n)
	}); err != nil || n != 0 {
		t.Fatal("snapshot tenant isolation failed")
	}
}
