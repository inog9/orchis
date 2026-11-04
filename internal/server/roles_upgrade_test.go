package server_test

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"orchis-go/internal/auth"
	"orchis-go/internal/bootstrap"
	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
	"os"
	"strings"
	"testing"
)

func TestThreeRolesUpgradeNeverWidensLegacyAccounts(t *testing.T) {
	env := testutil.NewAtVersion(t, 29)
	ctx := context.Background()
	frozen, err := os.ReadFile("testdata/schema29-roles.json")
	if err != nil {
		t.Fatal(err)
	}
	var roles []struct {
		Key, Name   string
		Permissions []string
	}
	if err = json.Unmarshal(frozen, &roles); err != nil {
		t.Fatal(err)
	}
	tenants := []uuid.UUID{ids.New(), ids.New()}
	type sample struct {
		id, tenant uuid.UUID
		old, want  string
		review     bool
	}
	samples := []sample{}
	for _, tenant := range tenants {
		if _, err = env.Owner.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,$2)`, tenant, tenant.String()); err != nil {
			t.Fatal(err)
		}
		for _, role := range roles {
			if _, err = env.Owner.Exec(ctx, `INSERT INTO roles(id,tenant_id,key,name,permissions,is_system) VALUES($1,$2,$3,$4,$5,true)`, ids.New(), tenant, role.Key, role.Name, role.Permissions); err != nil {
				t.Fatal(err)
			}
		}
		for _, spec := range []struct {
			key, want string
			perms     []string
			review    bool
		}{
			{"admin", "admin", nil, false}, {"analyst", "analyst", nil, false}, {"viewer", "viewer", nil, false},
			{"custom-full", "admin", roles[0].Permissions, false}, {"custom-reader", "viewer", roles[2].Permissions, false},
			{"findings-only", "viewer", []string{"finding.read"}, true},
			{"raw-report-not-effective", "viewer", []string{"finding.read", "report.read"}, true},
			{"empty", "viewer", []string{}, true},
		} {
			var rid uuid.UUID
			if spec.perms != nil {
				rid = ids.New()
				if _, err = env.Owner.Exec(ctx, `INSERT INTO roles(id,tenant_id,key,name,permissions) VALUES($1,$2,$3,$3,$4)`, rid, tenant, spec.key, spec.perms); err != nil {
					t.Fatal(err)
				}
			} else if err = env.Owner.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id=$1 AND key=$2`, tenant, spec.key).Scan(&rid); err != nil {
				t.Fatal(err)
			}
			uid := ids.New()
			if _, err = env.Owner.Exec(ctx, `INSERT INTO users(id,tenant_id,email,display_name,role_id) VALUES($1,$2,$3,'Legacy fixture',$4)`, uid, tenant, uid.String()+"@example.test", rid); err != nil {
				t.Fatal(err)
			}
			// Existing credentials must be revoked for accounts requiring review.
			if _, err = env.Owner.Exec(ctx, `INSERT INTO sessions(id,tenant_id,user_id,token_hash,idle_expires_at,absolute_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 day')`, ids.New(), tenant, uid, auth.HashToken(uid.String())); err != nil {
				t.Fatal(err)
			}
			if _, err = env.Owner.Exec(ctx, `INSERT INTO api_tokens(id,tenant_id,user_id,name,token_prefix,token_hash) VALUES($1,$2,$3,'fixture','fixture',$4)`, ids.New(), tenant, uid, auth.HashToken("api"+uid.String())); err != nil {
				t.Fatal(err)
			}
			samples = append(samples, sample{uid, tenant, spec.key, spec.want, spec.review})
		}
	}
	migration, err := os.ReadFile("../../migrations/00030_three_roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Single transaction, matching goose migration transaction semantics.
	tx, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		var role, status string
		var review, sessionRevoked, tokenRevoked bool
		if err = env.Owner.QueryRow(ctx, `SELECT r.key,u.status,u.role_review_required,s.revoked_at IS NOT NULL,a.revoked_at IS NOT NULL FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id JOIN sessions s ON s.tenant_id=u.tenant_id AND s.user_id=u.id JOIN api_tokens a ON a.tenant_id=u.tenant_id AND a.user_id=u.id WHERE u.tenant_id=$1 AND u.id=$2`, s.tenant, s.id).Scan(&role, &status, &review, &sessionRevoked, &tokenRevoked); err != nil {
			t.Fatal(err)
		}
		if role != s.want || review != s.review || sessionRevoked != s.review || tokenRevoked != s.review || (s.review && status != "disabled") {
			t.Fatalf("mapping %s: role=%s status=%s review=%v sessionRevoked=%v tokenRevoked=%v", s.old, role, status, review, sessionRevoked, tokenRevoked)
		}
		var n int
		if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND resource_id=$2 AND action='user.role_migrated'`, s.tenant, s.id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("audit %s count=%d err=%v", s.old, n, err)
		}
	}
	var count int
	env.Owner.QueryRow(ctx, `SELECT count(*) FROM roles`).Scan(&count)
	if count != 6 {
		t.Fatalf("three roles per tenant: %d", count)
	}
	for _, q := range []string{
		`UPDATE roles SET permissions=ARRAY['asset.read'] WHERE key='viewer'`,
		`UPDATE users SET status='active' WHERE role_review_required`,
		`UPDATE roles SET key='custom' WHERE key='viewer'`,
	} {
		if _, err = env.Owner.Exec(ctx, q); err == nil {
			t.Fatalf("unsafe change accepted: %s", q)
		}
	}
	// A subsequent bootstrap cannot broaden Viewer permissions.
	if _, err = bootstrap.EnsureTenant(ctx, env.App, tenants[0].String()); err != nil {
		t.Fatal(err)
	}
}

func TestMigratedAccountActivationRequiresPermissionReview(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	c := a.createUser(t, admin, "review@a.test", "viewer")
	if _, err := env.Owner.Exec(context.Background(), `UPDATE users SET role_review_required=true,previous_role_key='findings-only',status='disabled' WHERE id=$1`, c.User.ID); err != nil {
		t.Fatal(err)
	}
	var version int
	env.Owner.QueryRow(context.Background(), `SELECT version FROM users WHERE id=$1`, c.User.ID).Scan(&version)
	input := map[string]any{"version": version, "status": "active", "role": "viewer"}
	if r := a.do("PATCH", "/api/v1/users/"+c.User.ID, input, admin); r.Status != 422 {
		t.Fatalf("unreviewed activation: %d %s", r.Status, r.Body)
	}
	input["accept_role_permissions"] = true
	if r := a.do("PATCH", "/api/v1/users/"+c.User.ID, input, admin); r.Status != 200 {
		t.Fatalf("reviewed activation: %d %s", r.Status, r.Body)
	}
	var review bool
	env.Owner.QueryRow(context.Background(), `SELECT role_review_required FROM users WHERE id=$1`, c.User.ID).Scan(&review)
	if review {
		t.Fatal("review flag not cleared")
	}
}

func TestBootstrapDoesNotReplaceExistingUnprivilegedHuman(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	if _, err := bootstrap.CreateUser(ctx, env.App, env.TenantA, "narrow@example.test", "Narrow", "viewer", pw); err != nil {
		t.Fatal(err)
	}
	if created, _, err := bootstrap.EnsureAdmin(ctx, env.App, env.TenantA, "replacement@example.test", pw); err == nil || created {
		t.Fatalf("bootstrap silently created replacement administrator: %v %v", created, err)
	}
	if created, _, err := bootstrap.EnsureAdmin(ctx, env.App, env.TenantB, "fresh@example.test", pw); err != nil || !created {
		t.Fatalf("fresh tenant bootstrap: %v %v", created, err)
	}
}

func TestBootstrapOnlyRecognizesActiveRealHumanAdministrator(t *testing.T) {
	for _, kind := range []string{"disabled", "demo", "service"} {
		t.Run(kind, func(t *testing.T) {
			env := testutil.New(t)
			ctx := context.Background()
			uid, err := bootstrap.CreateUser(ctx, env.App, env.TenantA, "old@example.test", "Old", "admin", pw)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "disabled":
				_, err = env.Owner.Exec(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, uid)
			case "demo":
				_, err = env.Owner.Exec(ctx, `UPDATE users SET is_demo=true WHERE id=$1`, uid)
			case "service":
				_, err = env.Owner.Exec(ctx, `UPDATE users SET kind='service' WHERE id=$1`, uid)
			}
			if err != nil {
				t.Fatal(err)
			}
			made, _, err := bootstrap.EnsureAdmin(ctx, env.App, env.TenantA, "new@example.test", pw)
			if kind == "disabled" {
				if err == nil || made {
					t.Fatalf("inactive administrator was accepted or replaced: created=%v err=%v", made, err)
				}
			} else if err != nil || !made {
				t.Fatalf("%s administrator prevented fresh human bootstrap: created=%v err=%v", kind, made, err)
			}
		})
	}
}
