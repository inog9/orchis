// Package db membuka koneksi, menjalankan migrasi, dan menyediakan transaksi berkonteks tenant.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"orchis-go/migrations"
)

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*DB, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &DB{Pool: p}, nil
}

func (d *DB) Close() { d.Pool.Close() }

// InTenant menjalankan fn dalam satu transaksi dengan app.tenant_id terpasang, sehingga RLS berlaku.
// Tanpa konteks ini, kueri ke tabel ber-tenant tidak mengembalikan baris (gagal aman).
func (d *DB) InTenant(ctx context.Context, tenant uuid.UUID, fn func(tx pgx.Tx) error) error {
	return d.inTenant(ctx, tenant, pgx.TxOptions{}, fn)
}

// InTenantRead sama seperti InTenant tetapi hanya-baca.
func (d *DB) InTenantRead(ctx context.Context, tenant uuid.UUID, fn func(tx pgx.Tx) error) error {
	return d.inTenant(ctx, tenant, pgx.TxOptions{AccessMode: pgx.ReadOnly}, fn)
}

func (d *DB) inTenant(ctx context.Context, tenant uuid.UUID, opt pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := d.Pool.BeginTx(ctx, opt)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // tidak berdampak setelah Commit
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant.String()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Migrate menjalankan migrasi sebagai peran pemilik, lalu memberi hak akses tabel kepada peran aplikasi.
func Migrate(ctx context.Context, ownerURL, appRole string) error {
	if !identRe.MatchString(appRole) {
		return fmt.Errorf("nama peran aplikasi tidak valid: %q", appRole)
	}
	sdb, err := sql.Open("pgx", ownerURL)
	if err != nil {
		return err
	}
	defer sdb.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sdb, "."); err != nil {
		return fmt.Errorf("migrasi gagal: %w", err)
	}
	for _, q := range []string{
		fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %s`, appRole),
		fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %s`, appRole),
		fmt.Sprintf(`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %s`, appRole),
	} {
		if _, err := sdb.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("pemberian hak akses gagal: %w", err)
		}
	}
	return nil
}
