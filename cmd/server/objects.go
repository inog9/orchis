package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/blob"
	"orchis-go/internal/config"
	"orchis-go/internal/db"
)

// objectDigests verifies stored PDFs without a user password. Only the explicit
// operator CLI uses this application DB/S3 configuration; no endpoint exposes it.
func objectDigests(ctx context.Context, cfg *config.Config) (map[string]string, error) {
	out := map[string]string{}
	err := withTenant(ctx, cfg, func(d *db.DB, tenant uuid.UUID) error {
		store, e := blob.Open(ctx, blob.Options{Endpoint: cfg.S3.Endpoint, AccessKey: cfg.S3.AccessKey, SecretKey: cfg.S3.SecretKey, Bucket: cfg.S3.Bucket, TLS: cfg.S3.TLS})
		if e != nil {
			return e
		}
		return d.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
			rows, e := tx.Query(ctx, `SELECT id,pdf_key FROM reports WHERE tenant_id=$1 AND status='ready' ORDER BY id`, tenant)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				var key string
				if e = rows.Scan(&id, &key); e != nil {
					return e
				}
				if key == "" {
					return errors.New("ready report has no object")
				}
				content, e := store.Get(ctx, key)
				if e != nil {
					return e
				}
				if !bytes.HasPrefix(content, []byte("%PDF-")) {
					return errors.New("stored report is not PDF")
				}
				sum := sha256.Sum256(content)
				out[id.String()] = hex.EncodeToString(sum[:])
			}
			return rows.Err()
		})
	})
	return out, err
}
