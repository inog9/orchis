package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/ids"
)

// FreezeScanSnapshot dipanggil saat Scan terminal dalam transaksi yang sama dengan penutupan job.
func (s *Service) FreezeScanSnapshot(ctx context.Context, tx pgx.Tx, tenant, org, scan uuid.UUID, snap Snapshot) error {
	for i := range snap.Assets {
		a := &snap.Assets[i]
		if a.Discovered {
			continue
		}
		if err := func() error { var e error; a.Endpoint, e = endpointPosture(ctx, tx, tenant, *a); return e }(); err != nil {
			return err
		}
	}
	snap.EndpointSummary = summarizeEndpoint(snap.Assets)
	snap.Risk = exposureRisk(snap)
	snap.Organization = Redact(snap.Organization, 200)
	var previous []byte
	err := tx.QueryRow(ctx, `SELECT r.snapshot FROM reports r JOIN assessments a ON a.tenant_id=r.tenant_id AND a.id=r.assessment_id WHERE r.tenant_id=$1 AND r.organization_id=$2 AND a.is_scan AND a.profile=$3 AND a.created_at<$4 ORDER BY a.created_at DESC,a.id DESC LIMIT 1`, tenant, org, snap.Profile, snap.CreatedAt).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		var old Snapshot
		if err = json.Unmarshal(previous, &old); err != nil {
			return err
		}
		snap.Comparison = Compare(snap, old)
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	id := ids.New()
	tag, err := tx.Exec(ctx, `INSERT INTO reports(id,tenant_id,organization_id,assessment_id,snapshot) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,assessment_id) DO NOTHING`, id, tenant, org, scan, data)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return audit.Write(ctx, tx, audit.Entry{Tenant: tenant, Action: "report.freeze", ResourceType: "report", ResourceID: &id, Detail: map[string]any{"scan_id": scan, "organization_id": org, "schema_version": snap.SchemaVersion}})
}
