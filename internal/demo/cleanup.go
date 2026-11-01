package demo

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"orchis-go/internal/httpx"
	"strings"
	"time"
)

// queueCleanup locks report claims before recording exact, server-derived keys.
// Active renders must finish first; queued claims cannot race a committed purge.
func queueCleanup(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id,status,pdf_key FROM reports WHERE tenant_id=$1 AND is_demo ORDER BY id FOR UPDATE`, tenant)
	if err != nil {
		return err
	}
	keys := []string{}
	for rows.Next() {
		var id uuid.UUID
		var state, key string
		if err = rows.Scan(&id, &state, &key); err != nil {
			rows.Close()
			return err
		}
		if state == "rendering" {
			rows.Close()
			return httpx.Conflict("A demo PDF is rendering. Wait for it to finish before purging demo data.")
		}
		expected := tenant.String() + "/reports/" + id.String() + ".pdf"
		if key != "" && key != expected {
			rows.Close()
			return httpx.Conflict("A demo report references an unexpected object key. No data was removed.")
		}
		// Also covers a past put followed by a failed report status update.
		keys = append(keys, expected)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO demo_cleanup_tasks(tenant_id,kind,target) SELECT $1,'blob',unnest($2::text[]) ON CONFLICT DO NOTHING`, tenant, keys); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO demo_cleanup_tasks(tenant_id,kind,target) SELECT tenant_id,'search',id::text FROM findings WHERE tenant_id=$1 AND is_demo ON CONFLICT DO NOTHING`, tenant)
	return err
}

// CleanupOnce is idempotent even if external deletion succeeds and commit fails.
// Missing adapters leave durable tasks visible; they never imply cleanup success.
func (s *Service) CleanupOnce(ctx context.Context, tenant uuid.UUID) error {
	run, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for _, kind := range []string{"blob", "search"} {
		if kind == "blob" && s.Blob == nil || kind == "search" && s.Search == nil {
			continue
		}
		err := s.DB.InTenant(run, tenant, func(tx pgx.Tx) error {
			rows, err := tx.Query(run, `SELECT target FROM demo_cleanup_tasks WHERE tenant_id=$1 AND kind=$2 ORDER BY target LIMIT 64 FOR UPDATE SKIP LOCKED`, tenant, kind)
			if err != nil {
				return err
			}
			targets := []string{}
			for rows.Next() {
				var key string
				if err = rows.Scan(&key); err != nil {
					rows.Close()
					return err
				}
				targets = append(targets, key)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				return nil
			}
			switch kind {
			case "blob":
				prefix := tenant.String() + "/reports/"
				for _, key := range targets {
					if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, ".pdf") {
						return errors.New("invalid demo cleanup object key")
					}
					id, err := uuid.Parse(strings.TrimSuffix(strings.TrimPrefix(key, prefix), ".pdf"))
					if err != nil || key != prefix+id.String()+".pdf" {
						return errors.New("invalid demo cleanup object key")
					}
					if err = s.Blob.Remove(run, key); err != nil {
						return err
					}
				}
			case "search":
				ids := make([]uuid.UUID, 0, len(targets))
				for _, key := range targets {
					id, err := uuid.Parse(key)
					if err != nil {
						return errors.New("invalid demo cleanup finding id")
					}
					ids = append(ids, id)
				}
				if err = s.Search.RemoveFindings(run, tenant, ids); err != nil {
					return err
				}
			}
			_, err = tx.Exec(run, `DELETE FROM demo_cleanup_tasks WHERE tenant_id=$1 AND kind=$2 AND target=ANY($3::text[])`, tenant, kind, targets)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RunCleanup(ctx context.Context, tenant uuid.UUID) {
	reported := false
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := s.CleanupOnce(ctx, tenant); err != nil && ctx.Err() == nil {
				if !reported {
					slog.Warn("demo artifact cleanup pending", "tenant", tenant)
					reported = true
				}
			} else if err == nil {
				reported = false
			}
		}
	}
}
