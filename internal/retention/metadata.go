package retention

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Remove notification text and unpublished/published outbox payloads tied to
// expired/deleted objects. Shared object identities are deliberately not passed.
func removeMetadata(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, resources map[string][]uuid.UUID) error {
	for kind, ids := range resources {
		if len(ids) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM alerts WHERE tenant_id=$1 AND resource_type=$2 AND resource_id=ANY($3::uuid[])`, tenant, kind, ids); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM outbox WHERE tenant_id=$1 AND aggregate_type=$2 AND aggregate_id=ANY($3::uuid[])`, tenant, kind, ids); err != nil {
			return err
		}
	}
	return nil
}
