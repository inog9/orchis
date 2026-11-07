package reporting

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/alerts"
	"orchis-go/internal/audit"
	"orchis-go/internal/authz"
	"orchis-go/internal/ids"
	"orchis-go/internal/scancontrol"
)

// FreezeScanSnapshot dipanggil saat Scan terminal dalam transaksi yang sama dengan penutupan job.
func (s *Service) FreezeScanSnapshot(ctx context.Context, tx pgx.Tx, tenant, org, scan uuid.UUID, snap Snapshot) error {
	if err := scancontrol.Lock(ctx, tx, tenant); err != nil {
		return err
	}
	for i := range snap.Assets {
		a := &snap.Assets[i]
		if a.Discovered {
			continue
		}
		if err := func() error { var e error; a.Endpoint, e = endpointPosture(ctx, tx, tenant, *a); return e }(); err != nil {
			return err
		}
	}
	if err := tx.QueryRow(ctx, `SELECT report_language FROM assessments WHERE tenant_id=$1 AND id=$2`, tenant, scan).Scan(&snap.Language); err != nil {
		return err
	}
	snap.EndpointSummary = summarizeEndpoint(snap.Assets)
	snap.Risk = exposureRisk(snap)
	snap.Organization = Redact(snap.Organization, 200)
	if err := freezeComparison(ctx, tx, tenant, org, &snap, true); err != nil {
		return err
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
	kind, title, severity := "scan_finished", "Scan finished", "info"
	var incomplete bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2 AND status IN('failed','skipped','incomplete'))`, tenant, scan).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete {
		kind, title, severity = "scan_failed", "Scan finished with failed or skipped checks", "warning"
	}
	if snap.Status == "canceled" {
		kind, title, severity = "scan_stopped", "Scan was stopped", "warning"
	}
	if err = alerts.Raise(ctx, tx, tenant, alerts.New{Kind: kind, Severity: severity, Title: title, ResourceType: "scan", ResourceID: &scan, Audience: authz.AssessmentRead, DedupeKey: "scan-terminal:" + scan.String()}); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{Tenant: tenant, Action: "report.freeze", ResourceType: "report", ResourceID: &id, Detail: map[string]any{"scan_id": scan, "organization_id": org, "schema_version": snap.SchemaVersion}})
}
