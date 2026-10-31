package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
)

func TestMigrationsCreateSchema(t *testing.T) {
	env := testutil.New(t)
	var n int
	if err := env.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' AND table_name NOT LIKE 'goose%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 50 {
		t.Fatalf("diharapkan minimal 50 tabel, dapat %d", n)
	}
}

// Menjaga aturan ADR-006: tabel baru ber-tenant_id yang lupa memanggil enable_tenant_rls menggagalkan build.
func TestEveryTenantTableHasRLS(t *testing.T) {
	env := testutil.New(t)
	rows, err := env.Owner.Query(context.Background(), `
SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
       EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid = c.oid AND p.polname = 'tenant_isolation')
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped
WHERE n.nspname = 'public' AND c.relkind IN ('r','p') AND c.relispartition = false
ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var name string
		var rls, force, policy bool
		if err := rows.Scan(&name, &rls, &force, &policy); err != nil {
			t.Fatal(err)
		}
		count++
		if !rls || !force || !policy {
			t.Errorf("tabel %s: rls=%v force=%v policy=%v (panggil enable_tenant_rls di migrasi)", name, rls, force, policy)
		}
	}
	if count < 40 {
		t.Fatalf("hanya %d tabel ber-tenant terdeteksi; pemeriksaan tidak masuk akal", count)
	}
}

// Menjaga aturan "setiap kunci asing punya indeks pendukung" (pelajaran dari OpenAEV).
func TestEveryForeignKeyHasSupportingIndex(t *testing.T) {
	env := testutil.New(t)
	rows, err := env.Owner.Query(context.Background(), `
SELECT c.conrelid::regclass::text, pg_get_constraintdef(c.oid) FROM pg_constraint c
WHERE c.contype='f' AND NOT EXISTS (
  SELECT 1 FROM pg_index i WHERE i.indrelid=c.conrelid AND i.indisvalid
   AND (i.indkey::int2[])[0:cardinality(c.conkey)-1] @> c.conkey
   AND (i.indkey::int2[])[0:cardinality(c.conkey)-1] <@ c.conkey)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tbl, def string
		rows.Scan(&tbl, &def)
		t.Errorf("kunci asing tanpa indeks pendukung pada %s: %s", tbl, def)
	}
}

func TestRLSTenantIsolation(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	// tenant A menulis
	if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name) VALUES ($1,$2,'tag-a')`, ids.New(), env.TenantA)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var nA, nB, nNone int
	env.App.InTenantRead(ctx, env.TenantA, func(tx pgx.Tx) error { return tx.QueryRow(ctx, `SELECT count(*) FROM tags`).Scan(&nA) })
	env.App.InTenantRead(ctx, env.TenantB, func(tx pgx.Tx) error { return tx.QueryRow(ctx, `SELECT count(*) FROM tags`).Scan(&nB) })
	tx, _ := env.App.Pool.Begin(ctx)
	tx.QueryRow(ctx, `SELECT count(*) FROM tags`).Scan(&nNone)
	tx.Rollback(ctx)
	if nA != 1 || nB != 0 || nNone != 0 {
		t.Fatalf("isolasi salah: A=%d B=%d tanpa-konteks=%d (harapan 1/0/0)", nA, nB, nNone)
	}
	// tenant B tidak boleh menulis baris atas nama tenant A
	err := env.App.InTenant(ctx, env.TenantB, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name) VALUES ($1,$2,'curang')`, ids.New(), env.TenantA)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Fatalf("penulisan lintas tenant seharusnya ditolak RLS, dapat: %v", err)
	}
}

func TestCompositeFKRejectsCrossTenantReference(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	tagID, assetB := ids.New(), ids.New()
	if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name) VALUES ($1,$2,'a')`, tagID, env.TenantA)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// aset milik B; mencoba memakai tag milik A. Pemilik tabel dipakai agar yang diuji FK, bukan RLS.
	if _, err := env.Owner.Exec(ctx, `INSERT INTO assets (id, tenant_id, name, category) VALUES ($1,$2,'srv-b','host')`, assetB, env.TenantB); err != nil {
		t.Fatal(err)
	}
	_, err := env.Owner.Exec(ctx, `INSERT INTO asset_tags (tenant_id, asset_id, tag_id) VALUES ($1,$2,$3)`, env.TenantB, assetB, tagID)
	if err == nil || !strings.Contains(err.Error(), "foreign key") {
		t.Fatalf("referensi lintas tenant seharusnya ditolak FK komposit, dapat: %v", err)
	}
}
