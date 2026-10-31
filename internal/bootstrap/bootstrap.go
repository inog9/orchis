// Package bootstrap menyiapkan data awal: tenant, peran bawaan, dan admin pertama.
package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/groups"
	"orchis-go/internal/ids"
)

// EnsureTenant membuat tenant bila belum ada, lalu menyelaraskan peran bawaan dengan kode.
func EnsureTenant(ctx context.Context, d *db.DB, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := d.Pool.QueryRow(ctx, `
INSERT INTO tenants (id, name) VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`, ids.New(), name).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	err = d.InTenant(ctx, id, func(tx pgx.Tx) error {
		for _, r := range authz.SystemRoles {
			if _, err := tx.Exec(ctx, `
INSERT INTO roles (id, tenant_id, key, name, description, permissions, is_system)
VALUES ($1, $2, $3, $4, $5, $6, true)
ON CONFLICT (tenant_id, key) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description,
  permissions = EXCLUDED.permissions, updated_at = now()`,
				ids.New(), id, r.Key, r.Name, r.Description, r.Permissions); err != nil {
				return err
			}
		}
		if err := execution.SeedCatalog(ctx, tx, id); err != nil {
			return err
		}
		return groups.SeedSystem(ctx, tx, id)
	})
	return id, err
}

// CreateUser membuat akun login. Mengembalikan ID pengguna.
func CreateUser(ctx context.Context, d *db.DB, tenant uuid.UUID, email, displayName, roleKey, password string) (uuid.UUID, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	uid := ids.New()
	err = d.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO users (id, tenant_id, email, display_name, role_id, password_hash)
VALUES ($1, $2, $3, $4, (SELECT id FROM roles WHERE key = $5), $6)`, uid, tenant, email, displayName, roleKey, hash)
		return err
	})
	return uid, err
}

// EnsureAdmin membuat admin pertama bila belum ada admin sama sekali. Bila password kosong, dibuat acak
// dan dikembalikan agar dicatat sekali oleh pemanggil.
func EnsureAdmin(ctx context.Context, d *db.DB, tenant uuid.UUID, email, password string) (created bool, generated string, err error) {
	var n int
	if err = d.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id AND r.tenant_id = u.tenant_id
WHERE r.key = 'admin' AND u.deleted_at IS NULL`).Scan(&n)
	}); err != nil {
		return false, "", err
	}
	if n > 0 {
		return false, "", nil
	}
	if email == "" {
		return false, "", errors.New("ORCHIS_ADMIN_EMAIL wajib diisi untuk membuat admin pertama")
	}
	if password == "" {
		b := make([]byte, 18)
		if _, err = rand.Read(b); err != nil {
			return false, "", err
		}
		password = base64.RawURLEncoding.EncodeToString(b)
		generated = password
	}
	if _, err = CreateUser(ctx, d, tenant, email, "Administrator", "admin", password); err != nil {
		return false, "", err
	}
	slog.Info("admin pertama dibuat", "email", email)
	return true, generated, nil
}
