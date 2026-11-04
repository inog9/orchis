package retention

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/testutil"
	"testing"
)

func admin(t *testing.T, e *testutil.Env, tenant uuid.UUID) *auth.Principal {
	t.Helper()
	id := uuid.New()
	if _, err := e.Owner.Exec(context.Background(), `INSERT INTO users(id,tenant_id,email,display_name,role_id) SELECT $1,$2,$3,'Fixture',id FROM roles WHERE tenant_id=$2 AND key='admin'`, id, tenant, id.String()+"@fixture.test"); err != nil {
		t.Fatal(err)
	}
	return &auth.Principal{TenantID: tenant, UserID: id, Permissions: authz.SystemRoles[0].Permissions}
}
func problem(t *testing.T, err error, status int) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != status {
		t.Fatalf("expected HTTP %d, got %v", status, err)
	}
}
func TestRetentionPolicyCASMinimumAndTenantIsolation(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	s := Service{DB: e.App}
	current, err := s.Get(ctx, p)
	if err != nil || current != Defaults() {
		t.Fatalf("default policy: %+v %v", current, err)
	}
	var n int
	if err = e.Owner.QueryRow(ctx, `SELECT count(*) FROM retention_policies`).Scan(&n); err != nil || n != 0 {
		t.Fatal("read unexpectedly created policy")
	}
	updated, err := s.Update(ctx, p, current, "", "fixture")
	if err != nil || updated.Version != 1 {
		t.Fatalf("save policy: %+v %v", updated, err)
	}
	_, err = s.Update(ctx, p, current, "", "stale")
	problem(t, err, 409)
	invalid := updated
	invalid.AuditDays = MinimumAuditDays - 1
	_, err = s.Update(ctx, p, invalid, "", "invalid")
	problem(t, err, 422)
	invalid = updated
	invalid.EndpointDays = 0
	_, err = s.Update(ctx, p, invalid, "", "invalid")
	problem(t, err, 422)
	for _, role := range authz.SystemRoles[1:] {
		other := *p
		other.Permissions = role.Permissions
		_, err = s.Get(ctx, &other)
		problem(t, err, 403)
		_, err = s.Update(ctx, &other, updated, "", "forbidden")
		problem(t, err, 403)
	}
	if _, err = e.Owner.Exec(ctx, `UPDATE retention_policies SET audit_days=89 WHERE tenant_id=$1`, p.TenantID); err == nil {
		t.Fatal("database allowed audit retention below minimum")
	}
	if err = e.App.InTenantRead(ctx, e.TenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM retention_policies WHERE tenant_id=$1`, e.TenantA).Scan(&n)
	}); err != nil || n != 0 {
		t.Fatal("retention policy leaked across tenants")
	}
	if err = e.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='retention.policy.update'`, e.TenantA).Scan(&n); err != nil || n != 1 {
		t.Fatalf("policy audit count: %d %v", n, err)
	}
}

func TestWorkerMetadataDoesNotChangePolicyVersion(t *testing.T) {
	for _, kind := range []string{"sweep", "error"} {
		t.Run(kind, func(t *testing.T) {
			e := testutil.New(t)
			ctx := context.Background()
			p := admin(t, e, e.TenantA)
			s := Service{DB: e.App}
			before, err := s.Get(ctx, p)
			if err != nil || before.Version != 0 {
				t.Fatal("initial policy", before, err)
			}
			touch := func() {
				t.Helper()
				if kind == "sweep" {
					if err := s.Sweep(ctx, p.TenantID); err != nil {
						t.Fatal(err)
					}
				} else {
					s.recordError(ctx, p.TenantID, "retention_cleanup_pending")
				}
			}
			touch()
			after, err := s.Get(ctx, p)
			if err != nil || after.Version != before.Version {
				t.Fatalf("worker changed unsaved configuration version: %+v %v", after, err)
			}
			before.EndpointDays = 14
			saved, err := s.Update(ctx, p, before, "", "race")
			if err != nil || saved.Version != 1 {
				t.Fatalf("metadata caused false CAS conflict: %+v %v", saved, err)
			}
			touch()
			after, err = s.Get(ctx, p)
			if err != nil || after.Version != saved.Version || after.EndpointDays != 14 {
				t.Fatalf("worker overwrote saved configuration: %+v %v", after, err)
			}
		})
	}
}
