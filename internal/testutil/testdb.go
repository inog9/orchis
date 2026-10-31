// Package testutil menyediakan database PostgreSQL sungguhan per tes (tanpa mock).
package testutil

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"orchis-go/internal/auth"
	"orchis-go/internal/bootstrap"
	"orchis-go/internal/db"
)

type Env struct {
	Owner   *pgxpool.Pool // pemilik tabel: melewati RLS, hanya untuk menyiapkan/memeriksa data
	App     *db.DB        // peran aplikasi: tunduk pada RLS (seperti di produksi)
	TenantA uuid.UUID
	TenantB uuid.UUID
}

func init() {
	// Parameter murah agar tes cepat; produksi memakai nilai bawaan.
	auth.UseFastHashingForTests()
	// Log akses HTTP memenuhi keluaran tes; set ORCHIS_TEST_LOG=1 untuk menampilkannya.
	if os.Getenv("ORCHIS_TEST_LOG") == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
}

func adminURL() string {
	if v := os.Getenv("ORCHIS_TEST_PG_URL"); v != "" {
		return v
	}
	return "postgres://orchis:orchis@127.0.0.1:15432/postgres"
}

// New membuat database baru, menjalankan seluruh migrasi, dan menyiapkan dua tenant.
func New(t *testing.T) *Env {
	t.Helper()
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, adminURL())
	if err != nil {
		t.Fatalf("tidak bisa tersambung ke PostgreSQL tes (jalankan: make dev-up): %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		t.Fatalf("PostgreSQL tes tidak menjawab (jalankan: make dev-up): %v", err)
	}
	name := "orchis_test_" + strings.ReplaceAll(uuid.NewString()[:13], "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close()
	})
	ownerURL := strings.Replace(adminURL(), "/postgres", "/"+name, 1)
	appURL := fmt.Sprintf("postgres://orchis_app:orchis_app@%s/%s", hostOf(adminURL()), name)
	if err := db.Migrate(ctx, ownerURL, "orchis_app"); err != nil {
		t.Fatal(err)
	}
	owner, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	app, err := db.Open(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	a, err := bootstrap.EnsureTenant(ctx, app, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := bootstrap.EnsureTenant(ctx, app, "tenant-b")
	if err != nil {
		t.Fatal(err)
	}
	return &Env{Owner: owner, App: app, TenantA: a, TenantB: b}
}

func hostOf(u string) string {
	u = strings.TrimPrefix(u, "postgres://")
	u = u[strings.Index(u, "@")+1:]
	return u[:strings.Index(u, "/")]
}
