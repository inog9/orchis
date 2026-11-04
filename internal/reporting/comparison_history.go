package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// freezeComparison reads only immutable earlier reports in this tenant and organisation.
// Newest matching failures shadow older successes. Each tool may have a different baseline.
func freezeComparison(ctx context.Context, tx pgx.Tx, tenant, org uuid.UUID, snap *Snapshot, isScan bool) error {
	if err := tx.QueryRow(ctx, `SELECT findings_epoch FROM organizations WHERE tenant_id=$1 AND id=$2`, tenant, org).Scan(&snap.FindingsEpoch); err != nil {
		return err
	}
	b := NewComparison(*snap)
	rows, err := tx.Query(ctx, `SELECT r.id,r.snapshot FROM reports r JOIN assessments a ON a.tenant_id=r.tenant_id AND a.id=r.assessment_id WHERE r.tenant_id=$1 AND r.organization_id=$2 AND a.is_scan=$3 AND (a.created_at,a.id)<($4,$5) ORDER BY a.created_at DESC,a.id DESC`, tenant, org, isScan, snap.CreatedAt, snap.AssessmentID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			return err
		}
		var old Snapshot
		if err = json.Unmarshal(raw, &old); err != nil {
			return fmt.Errorf("invalid comparison snapshot: %w", err)
		}
		b.Add(old, id)
		if b.Done() {
			break
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	details := b.Result()
	snap.ComparisonDetails = &details
	snap.Comparison = details.Summary()
	return nil
}
