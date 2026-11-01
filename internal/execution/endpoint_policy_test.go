package execution

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"orchis-go/internal/db"
	"orchis-go/internal/osquery"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestEndpointPolicySerializesMissingSettingsAndAssetChanges(t *testing.T) {
	ctx := context.Background()
	adminURL := os.Getenv("ORCHIS_TEST_PG_URL")
	if adminURL == "" {
		adminURL = "postgres://orchis:orchis@127.0.0.1:15432/postgres"
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "orchis_policy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
	ownerURL := strings.Replace(adminURL, "/postgres", "/"+name, 1)
	if err = db.Migrate(ctx, ownerURL, "orchis_app"); err != nil {
		t.Fatal(err)
	}
	owner, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	appConfig, err := pgxpool.ParseConfig(ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	appConfig.ConnConfig.User = "orchis_app"
	appConfig.ConnConfig.Password = "orchis_app"
	app, err := db.Open(ctx, appConfig.ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	tenantA, tenantB := uuid.New(), uuid.New()
	if _, err = owner.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'a'),($2,'b')`, tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	org, asset := uuid.New(), uuid.New()
	if _, err := owner.Exec(ctx, `INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'policy','policy')`, org, tenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO assets(id,tenant_id,name,category,organization_id) VALUES($1,$2,'policy','host',$3)`, asset, tenantA, org); err != nil {
		t.Fatal(err)
	}
	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	go func() {
		done <- app.InTenant(ctx, tenantA, func(tx pgx.Tx) error {
			policy, _, err := currentEndpointPolicy(ctx, tx, tenantA, asset, osquery.Find("osquery.inventory"))
			if err != nil {
				close(locked)
				return err
			}
			if policy.Revision != 0 || policy.Organization == nil || *policy.Organization != org {
				close(locked)
				return errors.New("snapshot pengaturan kosong tidak benar")
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	for _, sql := range []string{
		`SELECT id FROM organizations WHERE id=$1 FOR UPDATE`,
		`UPDATE assets SET testing_authorized=false WHERE organization_id=$1`,
	} {
		tx, err := owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='200ms'`); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, sql, org)
		tx.Rollback(ctx)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "55P03" {
			t.Fatalf("perubahan harus menunggu snapshot: %v", err)
		}
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := app.InTenant(ctx, tenantB, func(tx pgx.Tx) error {
		policy, enabled, err := currentEndpointPolicy(ctx, tx, tenantB, asset, osquery.Find("osquery.inventory"))
		if err != nil {
			return err
		}
		if enabled || policy.Organization != nil {
			return errors.New("policy tenant lain bocor")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
