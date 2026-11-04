package retention

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

// CleanupOnce only derives exact keys from typed tenant/UUID intents. External
// success followed by rollback is retried safely; a missing adapter leaves work pending.
func (s *Service) CleanupOnce(ctx context.Context, tenant uuid.UUID) error {
	run, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for _, kind := range []string{"report_pdf", "execution_raw", "search_finding"} {
		if kind == "search_finding" && s.Search == nil || kind != "search_finding" && s.Blob == nil {
			continue
		}
		err := s.DB.InTenant(run, tenant, func(tx pgx.Tx) error {
			rows, err := tx.Query(run, `SELECT object_id FROM data_cleanup_tasks WHERE tenant_id=$1 AND kind=$2 ORDER BY object_id LIMIT 64 FOR UPDATE SKIP LOCKED`, tenant, kind)
			if err != nil {
				return err
			}
			var ids []uuid.UUID
			for rows.Next() {
				var id uuid.UUID
				if err = rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return nil
			}
			if kind == "search_finding" {
				if err = s.Search.RemoveFindings(run, tenant, ids); err != nil {
					return err
				}
			} else {
				for _, id := range ids {
					key := tenant.String() + "/reports/" + id.String() + ".pdf"
					if kind == "execution_raw" {
						key = "executions/" + tenant.String() + "/" + id.String() + "/output.txt"
					}
					if err = s.Blob.Remove(run, key); err != nil {
						return err
					}
				}
			}
			_, err = tx.Exec(run, `DELETE FROM data_cleanup_tasks WHERE tenant_id=$1 AND kind=$2 AND object_id=ANY($3::uuid[])`, tenant, kind, ids)
			return err
		})
		if err != nil {
			return err
		}
	}
	return s.DB.InTenant(run, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(run, `UPDATE data_deletions d SET completed_at=now() WHERE tenant_id=$1 AND completed_at IS NULL AND NOT EXISTS(SELECT 1 FROM data_cleanup_tasks t WHERE t.tenant_id=d.tenant_id AND t.operation_id=d.id)`, tenant)
		return err
	})
}
