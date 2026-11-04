package alerts

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/authz"
	"orchis-go/internal/osquery"
	"strconv"
)

func PendingApprovals(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	c := osquery.Consent()
	rows, e := tx.Query(ctx, `SELECT a.id,a.version FROM assets a WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND NOT a.is_demo AND NOT a.testing_authorized AND a.organization_id IS NOT NULL AND EXISTS(SELECT 1 FROM agents g WHERE g.tenant_id=a.tenant_id AND g.asset_id=a.id AND g.status<>'revoked' AND NOT g.is_demo) AND NOT EXISTS(SELECT 1 FROM alerts z WHERE z.tenant_id=a.tenant_id AND z.dedupe_key='authorization-pending:'||a.id::text||':'||a.version::text) ORDER BY a.id LIMIT 100`, tenant)
	if e != nil {
		return e
	}
	type item struct {
		id      uuid.UUID
		version int
	}
	items := []item{}
	for rows.Next() {
		var i item
		if e = rows.Scan(&i.id, &i.version); e != nil {
			rows.Close()
			return e
		}
		items = append(items, i)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, i := range items {
		if e = Raise(ctx, tx, tenant, New{Kind: "authorization_pending", Severity: "warning", Title: "Asset authorization needs administrator review", ResourceType: "asset", ResourceID: &i.id, Audience: authz.AssetAuthorize, DedupeKey: "authorization-pending:" + i.id.String() + ":" + strconv.Itoa(i.version)}); e != nil {
			return e
		}
	}
	rows, e = tx.Query(ctx, `SELECT DISTINCT s.organization_id FROM osquery_org_settings s JOIN organizations o ON o.tenant_id=s.tenant_id AND o.id=s.organization_id AND NOT o.is_demo WHERE s.tenant_id=$1 AND s.enabled AND (s.consent_version IS DISTINCT FROM $2 OR s.consent_sha256 IS DISTINCT FROM $3 OR s.consented_at IS NULL OR s.consented_by IS NULL) AND NOT EXISTS(SELECT 1 FROM alerts z WHERE z.tenant_id=s.tenant_id AND z.dedupe_key='consent-pending:'||s.organization_id::text||':'||$3) ORDER BY s.organization_id LIMIT 100`, tenant, c.Version, c.SHA256)
	if e != nil {
		return e
	}
	orgs := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		orgs = append(orgs, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, id := range orgs {
		if e = Raise(ctx, tx, tenant, New{Kind: "consent_pending", Severity: "warning", Title: "Endpoint consent needs administrator renewal", ResourceType: "organization", ResourceID: &id, Audience: authz.SettingsManage, DedupeKey: "consent-pending:" + id.String() + ":" + c.SHA256}); e != nil {
			return e
		}
	}
	return nil
}
func ReportEvent(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, status string, attempt int) error {
	var generation int
	if e := tx.QueryRow(ctx, `SELECT render_generation FROM reports WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&generation); e != nil {
		return e
	}
	kind, title, severity := "report_ready", "Report PDF is ready", "info"
	if status == "failed" {
		kind, title, severity = "pdf_failed", "Report PDF failed; retry is available", "warning"
	}
	return Raise(ctx, tx, tenant, New{Kind: kind, Severity: severity, Title: title, ResourceType: "report", ResourceID: &id, Audience: authz.ReportRead, DedupeKey: kind + ":" + id.String() + ":" + strconv.Itoa(generation) + ":" + strconv.Itoa(attempt)})
}
