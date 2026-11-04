package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/scancontrol"
)

// Rekey is an operator-only maintenance operation. API and workers must be
// stopped; the caller installs the new key only after this transaction commits.
// Already-reencrypted rows allow recovery after a committed DB change but a
// failed filesystem promotion. No credentials leave this function.
func (s *Service) Rekey(ctx context.Context, next []byte) (int, error) {
	if len(s.Key) != 32 || len(next) != 32 {
		return 0, errors.New("two valid AI master keys are required")
	}
	count := 0
	err := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, s.Tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "ai-rekey/"+s.Tenant.String()); err != nil {
			return err
		}
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM ai_jobs WHERE tenant_id=$1 AND status IN('queued','running')`, s.Tenant).Scan(&active); err != nil {
			return err
		}
		if active != 0 {
			return errors.New("finish or cancel queued/running AI jobs before rekey")
		}
		rows, err := tx.Query(ctx, `SELECT organization_id,secrets_enc FROM ai_policies WHERE tenant_id=$1 AND secrets_enc IS NOT NULL ORDER BY organization_id FOR UPDATE`, s.Tenant)
		if err != nil {
			return err
		}
		type entry struct {
			id   uuid.UUID
			data []byte
		}
		entries := []entry{}
		for rows.Next() {
			var e entry
			if err = rows.Scan(&e.id, &e.data); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			aad := s.Tenant.String() + "/" + e.id.String()
			if already, err := Open(next, e.data, aad); err == nil {
				clear(already)
				continue
			} // safe retry, or same-key no-op
			plain, err := Open(s.Key, e.data, aad)
			if err != nil {
				return errors.New("AI master key mismatch; no policy was changed")
			}
			sealed, err := Seal(next, plain, aad)
			clear(plain)
			if err != nil {
				return errors.New("AI re-encryption failed")
			}
			if _, err = tx.Exec(ctx, `UPDATE ai_policies SET secrets_enc=$3,version=version+1,updated_at=now() WHERE tenant_id=$1 AND organization_id=$2`, s.Tenant, e.id, sealed); err != nil {
				return err
			}
			count++
		}
		if count == 0 || bytes.Equal(s.Key, next) {
			return nil
		}
		oldHash, newHash := sha256.Sum256(s.Key), sha256.Sum256(next)
		return audit.Write(ctx, tx, audit.Entry{Tenant: s.Tenant, Action: "ai.master_key_rotated", ResourceType: "ai", Detail: map[string]any{"policies": count, "old_key_fingerprint": hex.EncodeToString(oldHash[:]), "new_key_fingerprint": hex.EncodeToString(newHash[:])}})
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
