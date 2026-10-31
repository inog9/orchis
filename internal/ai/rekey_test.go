package ai

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/testutil"
	"testing"
)

func TestMasterRekeyTenantIsolationRecoveryAndRollback(t *testing.T) {
	ctx := context.Background()
	env := testutil.New(t)
	old, next := bytes.Repeat([]byte{11}, 32), bytes.Repeat([]byte{22}, 32)
	tenants := []uuid.UUID{env.TenantA, env.TenantA, env.TenantB}
	orgs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	if orgs[0].String() > orgs[1].String() {
		orgs[0], orgs[1] = orgs[1], orgs[0]
	}
	original := map[uuid.UUID][]byte{}
	for i, tenant := range tenants {
		org := orgs[i]
		cipher, err := Seal(old, []byte(`{"deepseek":"local-fake-only"}`), tenant.String()+"/"+org.String())
		if err != nil {
			t.Fatal(err)
		}
		original[org] = cipher
		if _, err = env.Owner.Exec(ctx, `INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'LOCAL REKEY FIXTURE',$3)`, org, tenant, org.String()); err != nil {
			t.Fatal(err)
		}
		if _, err = env.Owner.Exec(ctx, `INSERT INTO ai_policies(tenant_id,organization_id,secrets_enc) VALUES($1,$2,$3)`, tenant, org, cipher); err != nil {
			t.Fatal(err)
		}
	}
	svc := &Service{DB: env.App, Tenant: env.TenantA, Key: old}
	if n, err := svc.Rekey(ctx, next); err != nil || n != 2 {
		t.Fatalf("rotation count %d err %v", n, err)
	}
	for i, tenant := range tenants {
		var cipher []byte
		if err := env.Owner.QueryRow(ctx, `SELECT secrets_enc FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`, tenant, orgs[i]).Scan(&cipher); err != nil {
			t.Fatal(err)
		}
		expected := next
		if tenant == env.TenantB {
			expected = old
			if !bytes.Equal(cipher, original[orgs[i]]) {
				t.Fatal("foreign tenant changed")
			}
		}
		plain, err := Open(expected, cipher, tenant.String()+"/"+orgs[i].String())
		if err != nil || !bytes.Contains(plain, []byte("local-fake-only")) {
			t.Fatal("credential changed")
		}
		clear(plain)
	}
	if n, err := svc.Rekey(ctx, next); err != nil || n != 0 {
		t.Fatal("committed DB rotation cannot resume with old key still on disk", n, err)
	}
	var auditCount int
	if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='ai.master_key_rotated'`, env.TenantA).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("rotation audit not idempotent")
	}
	// Corrupt one policy and verify an attempted next rotation changes neither row.
	if _, err := env.Owner.Exec(ctx, `UPDATE ai_policies SET secrets_enc='broken'::bytea WHERE tenant_id=$1 AND organization_id=$2`, env.TenantA, orgs[1]); err != nil {
		t.Fatal(err)
	}
	var before []byte
	env.Owner.QueryRow(ctx, `SELECT secrets_enc FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`, env.TenantA, orgs[0]).Scan(&before)
	svc.Key = next
	if n, err := svc.Rekey(ctx, bytes.Repeat([]byte{33}, 32)); err == nil || n != 0 {
		t.Fatal("corrupt policy accepted")
	}
	var after []byte
	env.Owner.QueryRow(ctx, `SELECT secrets_enc FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`, env.TenantA, orgs[0]).Scan(&after)
	if !bytes.Equal(before, after) {
		t.Fatal("failed rotation partially committed")
	}
	if _, err := svc.Rekey(ctx, nil); err == nil {
		t.Fatal("invalid key accepted")
	}
}
