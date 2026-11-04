package demo_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"orchis-go/internal/integrations"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/agents"
	"orchis-go/internal/auth"
	"orchis-go/internal/demo"
	"orchis-go/internal/execution"
	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
	"orchis-go/internal/users"
)

func TestRelatedFixturesRoundTripAndNoUsableCredentials(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error { return execution.SeedCatalog(ctx, tx, env.TenantA) }); err != nil {
		t.Fatal(err)
	}
	tables := []string{"roles", "users", "api_tokens", "password_reset_links", "people", "teams", "team_members", "asset_groups", "asset_group_members", "atomic_tests", "action_presets", "integrations", "scenario_teams", "scenario_targets"}
	baseline := map[string]int{}
	for _, table := range tables {
		baseline[table] = count(t, env, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, env.TenantA)
	}
	svc := &demo.Service{DB: env.App, Blob: emptyArtifacts{}, Search: emptyArtifacts{}}
	for i := 0; i < 2; i++ {
		if err := svc.Seed(ctx, env.TenantA, 7, nil, "", "related"); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, env, `SELECT count(*) FROM roles WHERE tenant_id=$1`, env.TenantA); n != 3 {
		t.Fatalf("expected exactly three built-in roles, got %d", n)
	}
	if n := count(t, env, `SELECT count(*) FROM executions WHERE tenant_id=$1 AND (NOT is_demo OR state<>'ended' OR raw_output_key IS NOT NULL)`, env.TenantA); n != 0 {
		t.Fatal("demo created dispatchable or unmarked execution")
	}
	st, err := svc.Status(ctx, env.TenantA)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"users", "api_tokens", "password_reset_links", "people", "teams", "asset_groups", "atomic_tests", "action_presets", "integrations"} {
		if st.Counts[table] == 0 {
			t.Fatalf("missing demo %s: %+v", table, st)
		}
	}
	if n := count(t, env, `SELECT count(*) FROM users WHERE tenant_id=$1 AND is_demo AND (status<>'disabled' OR password_hash IS NOT NULL)`, env.TenantA); n != 0 {
		t.Fatal("usable user fixture")
	}
	if n := count(t, env, `SELECT count(*) FROM api_tokens WHERE tenant_id=$1 AND is_demo AND (revoked_at IS NULL OR expires_at>now())`, env.TenantA); n != 0 {
		t.Fatal("usable token fixture")
	}
	if n := count(t, env, `SELECT count(*) FROM password_reset_links WHERE tenant_id=$1 AND is_demo AND (used_at IS NULL OR expires_at>now())`, env.TenantA); n != 0 {
		t.Fatal("usable reset fixture")
	}
	if n := count(t, env, `SELECT count(*) FROM asset_addresses ad JOIN assets a ON a.tenant_id=ad.tenant_id AND a.id=ad.asset_id WHERE a.tenant_id=$1 AND a.is_demo AND a.testing_authorized AND ad.value<>'127.0.0.1'`, env.TenantA); n != 0 {
		t.Fatal("authorized non-local demo address")
	}
	if n := count(t, env, `SELECT count(*) FROM asset_groups g LEFT JOIN asset_group_members m ON m.tenant_id=g.tenant_id AND m.asset_group_id=g.id WHERE g.tenant_id=$1 AND g.is_demo AND m.asset_id IS NULL`, env.TenantA); n != 0 {
		t.Fatal("unintentionally empty dynamic demo group")
	}
	for _, table := range []string{"outbox", "sessions"} {
		if n := count(t, env, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, env.TenantA); n != 0 {
			t.Fatalf("demo unexpectedly made %s", table)
		}
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "related"); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if got := count(t, env, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1`, env.TenantA); got != baseline[table] {
			t.Errorf("baseline %s: %d -> %d", table, baseline[table], got)
		}
	}
}

// Defense in depth: changing visible account status or replacing fixture hashes must
// still never turn demo identities/tokens into authenticated application principals.
func TestDemoIdentityAuthenticationRejectedEvenAfterCredentialChanges(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "auth"); err != nil {
		t.Fatal(err)
	}
	var user uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA).Scan(&user); err != nil {
		t.Fatal(err)
	}
	password := "Known-Fixture-Password-123"
	pw, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE users SET status='active',password_hash=$3 WHERE tenant_id=$1 AND id=$2`, env.TenantA, user, pw); err != nil {
		t.Fatal(err)
	}
	var email string
	env.Owner.QueryRow(ctx, `SELECT email::text FROM users WHERE id=$1`, user).Scan(&email)
	authSvc := auth.NewService(env.App)
	if _, err := authSvc.Login(ctx, env.TenantA, email, password, "127.0.0.1", "fixture", false); err == nil {
		t.Fatal("demo login accepted")
	}
	token := "oat_SYNTHETIC_public_fixture_only"
	if _, err := env.Owner.Exec(ctx, `UPDATE api_tokens SET token_hash=$3,revoked_at=NULL,expires_at=now()+interval '1 hour' WHERE tenant_id=$1 AND user_id=$2`, env.TenantA, user, auth.HashToken(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.AuthenticateToken(ctx, env.TenantA, token); err == nil {
		t.Fatal("demo user's API token accepted")
	}
	session := env.TenantA.String() + ".SYNTHETIC_public_fixture_only"
	if _, err := env.Owner.Exec(ctx, `INSERT INTO sessions(id,tenant_id,user_id,token_hash,idle_expires_at,absolute_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, ids.New(), env.TenantA, user, auth.HashToken(session)); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.Authenticate(ctx, session); err == nil {
		t.Fatal("demo session accepted")
	}
	reset := "SYNTHETIC_public_reset_fixture_only"
	if _, err := env.Owner.Exec(ctx, `UPDATE password_reset_links SET token_hash=$3,used_at=NULL,expires_at=now()+interval '1 hour' WHERE tenant_id=$1 AND user_id=$2`, env.TenantA, user, auth.HashToken(reset)); err != nil {
		t.Fatal(err)
	}
	us := &users.Service{DB: env.App, Tenant: env.TenantA}
	if err := us.CheckLink(ctx, reset); err == nil {
		t.Fatal("demo reset considered valid")
	}
	if err := us.UseLink(ctx, reset, "Another-Fixture-Password-123", "127.0.0.1"); err == nil {
		t.Fatal("demo reset accepted")
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE api_tokens SET is_demo=false WHERE tenant_id=$1 AND user_id=$2`, env.TenantA, user); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.AuthenticateToken(ctx, env.TenantA, token); err == nil {
		t.Fatal("real-marked token authenticates demo user")
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE api_tokens SET is_demo=true WHERE tenant_id=$1 AND user_id=$2`, env.TenantA, user); err != nil {
		t.Fatal(err)
	}
	// Isolate the token marker as well: even a real principal cannot use a demo token.
	if _, err := env.Owner.Exec(ctx, `UPDATE users SET is_demo=false WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.AuthenticateToken(ctx, env.TenantA, token); err == nil {
		t.Fatal("demo-marked API token accepted")
	}
	var agent uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM agents WHERE tenant_id=$1 AND is_demo AND status<>'revoked' LIMIT 1`, env.TenantA).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	secret := "oag_SYNTHETIC_public_agent_fixture_only"
	if _, err := env.Owner.Exec(ctx, `UPDATE agents SET secret_hash=$3,last_heartbeat_at=now()-interval '5 minutes' WHERE tenant_id=$1 AND id=$2`, env.TenantA, agent, auth.HashToken(secret)); err != nil {
		t.Fatal(err)
	}
	ag := &agents.Service{DB: env.App, Tenant: env.TenantA, Now: time.Now}
	if _, err := ag.Authenticate(ctx, secret); err == nil {
		t.Fatal("demo agent accepted")
	}
	if n, err := ag.Sweep(ctx); err != nil || n != 0 {
		t.Fatalf("sweep affected synthetic fixtures: %d %v", n, err)
	}
}

func TestPurgeRejectsRealMembersOfDemoTeam(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "members"); err != nil {
		t.Fatal(err)
	}
	person := ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO people(id,tenant_id,first_name) VALUES($1,$2,'Real person')`, person, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO team_members(tenant_id,team_id,person_id) SELECT tenant_id,id,$2 FROM teams WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA, person); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "members"); err == nil || !strings.Contains(err.Error(), "orang nyata") {
		t.Fatalf("real membership not protected: %v", err)
	}
	if n := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND is_demo`, env.TenantA); n != 50 {
		t.Fatal("partial purge")
	}
}

func TestSyntheticIntegrationStatusIsInertAndStable(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "integrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE integrations SET last_heartbeat_at=now()-interval '2 days' WHERE tenant_id=$1 AND is_demo`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	list, err := (&integrations.Service{DB: env.App}).List(ctx, &auth.Principal{TenantID: env.TenantA})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, i := range list {
		if i.IsDemo {
			found[i.Status] = true
			if i.HasToken {
				t.Fatal("demo worker has a token")
			}
		}
	}
	if len(found) != 3 {
		t.Fatalf("missing illustrative status: %+v", found)
	}
	if _, err := execution.IssueToken(ctx, env.App, env.TenantA, "demo.nmap.online"); err == nil {
		t.Fatal("demo worker credential issued")
	}
	token := "oij_SYNTHETIC_public_injector_fixture_only"
	sum := sha256.Sum256([]byte(token))
	if _, err := env.Owner.Exec(ctx, `UPDATE integrations SET config=jsonb_build_object('token_sha256',$2::text) WHERE tenant_id=$1 AND key='demo.nmap.online'`, env.TenantA, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if _, err := (&execution.Runtime{DB: env.App, Tenant: env.TenantA}).Authenticate(ctx, token); err == nil {
		t.Fatal("synthetic integration authenticated")
	}
}

func TestPurgePreservesRealRunAfterDemoTestRetargetedToRealAsset(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 2, nil, "", "retarget"); err != nil {
		t.Fatal(err)
	}
	asset := ids.New()
	exec := ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO assets(id,tenant_id,name,category) VALUES($1,$2,'Real asset','host')`, asset, env.TenantA); err != nil {
		t.Fatal(err)
	}
	var test uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT t.id FROM atomic_tests t JOIN contracts c ON c.tenant_id=t.tenant_id AND c.id=t.contract_id WHERE t.tenant_id=$1 AND t.is_demo AND c.key='nmap.tcp_connect' LIMIT 1`, env.TenantA).Scan(&test); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE atomic_tests SET target_asset_id=$3 WHERE tenant_id=$1 AND id=$2`, env.TenantA, test, asset); err != nil {
		t.Fatal(err)
	}
	// A non-demo control record stands for an operator's normal run. This test
	// never starts a process or sends a dispatch message.
	if _, err := env.Owner.Exec(ctx, `INSERT INTO executions(id,tenant_id,atomic_test_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,state,outcome,params,input_hash)
 SELECT $1,$2,t.id,t.contract_id,t.contract_version,$4,'127.0.0.1','injector',c.integration_id,'ended','error',t.params,decode('00','hex')
 FROM atomic_tests t JOIN contracts c ON c.tenant_id=t.tenant_id AND c.id=t.contract_id WHERE t.tenant_id=$2 AND t.id=$3`, exec, env.TenantA, test, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "retarget"); err == nil || !strings.Contains(err.Error(), "eksekusi nyata pada tes demo") {
		t.Fatalf("real run not protected: %v", err)
	}
	if n := count(t, env, `SELECT count(*) FROM executions WHERE tenant_id=$1 AND id=$2`, env.TenantA, exec); n != 1 {
		t.Fatal("operator run was cascaded")
	}
	if n := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND is_demo`, env.TenantA); n != 50 {
		t.Fatal("partial purge after real-reference refusal")
	}
}
