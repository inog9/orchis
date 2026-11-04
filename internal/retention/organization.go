package retention

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/scancontrol"
	"reflect"
	"time"
)

type Counts struct {
	Assets           int `json:"assets"`
	Agents           int `json:"agents"`
	Findings         int `json:"findings"`
	Reports          int `json:"reports"`
	EndpointDatasets int `json:"endpoint_datasets"`
	Scans            int `json:"scans"`
	Executions       int `json:"executions"`
}
type Preview struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Version  int       `json:"version"`
	Counts   Counts    `json:"counts"`
	Blockers []string  `json:"blockers"`
}
type DeleteInput struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Counts  Counts `json:"counts"`
}
type Deletion struct {
	ID               uuid.UUID  `json:"id"`
	OrganizationID   uuid.UUID  `json:"organization_id"`
	OrganizationName string     `json:"organization_name"`
	Counts           Counts     `json:"counts"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	Pending          int        `json:"cleanup_pending"`
	Status           string     `json:"status"`
}

func deletePermission(p *auth.Principal) error {
	return permission(p, authz.OrganizationWrite, authz.AssetDelete, authz.AgentManage, authz.FindingRead, authz.ReportRead)
}
func preview(ctx context.Context, tx pgx.Tx, tenant, org uuid.UUID, lock bool) (Preview, error) {
	out := Preview{ID: org, Blockers: []string{}}
	var demo bool
	query := `SELECT name,version,is_demo FROM organizations WHERE tenant_id=$1 AND id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	err := tx.QueryRow(ctx, query, tenant, org).Scan(&out.Name, &out.Version, &demo)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, httpx.NotFound("Organization")
	}
	if err != nil {
		return out, err
	}
	if demo {
		out.Blockers = append(out.Blockers, "Demo data is read-only; use demo purge.")
	}
	if lock {
		if _, err = tx.Exec(ctx, `SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2 ORDER BY id FOR UPDATE`, tenant, org); err != nil {
			return out, err
		}
	}
	err = tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM assets WHERE tenant_id=$1 AND organization_id=$2),
 (SELECT count(*) FROM agents WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2))),
 (SELECT count(*) FROM findings WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2))),
 (SELECT count(*) FROM reports WHERE tenant_id=$1 AND organization_id=$2),
 (SELECT count(*) FROM endpoint_datasets WHERE tenant_id=$1 AND asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2)),
 (SELECT count(*) FROM assessments WHERE tenant_id=$1 AND organization_id=$2),
 (SELECT count(*) FROM executions WHERE tenant_id=$1 AND (scan_organization_id=$2 OR target_asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2)))`, tenant, org).Scan(&out.Counts.Assets, &out.Counts.Agents, &out.Counts.Findings, &out.Counts.Reports, &out.Counts.EndpointDatasets, &out.Counts.Scans, &out.Counts.Executions)
	if err != nil {
		return out, err
	}
	var rendering, protected bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reports WHERE tenant_id=$1 AND organization_id=$2 AND status='rendering')`, tenant, org).Scan(&rendering); err != nil {
		return out, err
	}
	if rendering {
		out.Blockers = append(out.Blockers, "A PDF is rendering. Wait for it to finish or recover before deleting this organisation.")
	}
	// Historical runs of another organisation must not be cascaded when assets move.
	if err = tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM assessment_targets t JOIN assessments a ON a.tenant_id=t.tenant_id AND a.id=t.assessment_id JOIN assets x ON x.tenant_id=t.tenant_id AND x.id=t.asset_id WHERE t.tenant_id=$1 AND x.organization_id=$2 AND a.organization_id<>$2)
 OR EXISTS(SELECT 1 FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id JOIN assets x ON x.tenant_id=j.tenant_id AND (x.id=j.control_asset_id OR x.id=j.asset_id) WHERE j.tenant_id=$1 AND x.organization_id=$2 AND a.organization_id<>$2)
 OR EXISTS(SELECT 1 FROM simulation_targets t JOIN assets x ON x.tenant_id=t.tenant_id AND x.id=t.asset_id WHERE t.tenant_id=$1 AND x.organization_id=$2 AND t.scan_organization_id IS NOT NULL AND t.scan_organization_id<>$2)
 OR EXISTS(SELECT 1 FROM executions e JOIN assets x ON x.tenant_id=e.tenant_id AND x.id=e.target_asset_id WHERE e.tenant_id=$1 AND x.organization_id=$2 AND e.scan_organization_id IS NOT NULL AND e.scan_organization_id<>$2)
 OR EXISTS(SELECT 1 FROM ai_job_actions ja JOIN ai_jobs j ON j.tenant_id=ja.tenant_id AND j.id=ja.job_id JOIN assets x ON x.tenant_id=ja.tenant_id AND x.id=ja.asset_id WHERE ja.tenant_id=$1 AND x.organization_id=$2 AND j.organization_id<>$2)`, tenant, org).Scan(&protected); err != nil {
		return out, err
	}
	if protected {
		out.Blockers = append(out.Blockers, "An asset is referenced by another organisation's retained run. Remove that dependency before deleting; other organisations will not be changed.")
	}
	var crossReference bool
	if err = tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM network_range_authorizations n JOIN assets a ON a.tenant_id=n.tenant_id AND a.id=n.control_asset_id WHERE n.tenant_id=$1 AND a.organization_id=$2 AND n.organization_id<>$2)
 OR EXISTS(SELECT 1 FROM reports r JOIN assets a ON a.tenant_id=r.tenant_id AND a.organization_id=$2 WHERE r.tenant_id=$1 AND r.organization_id<>$2 AND r.snapshot @> jsonb_build_object('assets',jsonb_build_array(jsonb_build_object('id',a.id))))
 OR EXISTS(SELECT 1 FROM ai_jobs j JOIN reports r ON r.tenant_id=j.tenant_id AND r.id=j.report_id WHERE j.tenant_id=$1 AND r.organization_id=$2 AND j.organization_id<>$2)
 OR EXISTS(SELECT 1 FROM assessment_targets t JOIN assessments a ON a.tenant_id=t.tenant_id AND a.id=t.assessment_id WHERE t.tenant_id=$1 AND a.organization_id<>$2 AND t.simulation_id IN(SELECT t2.simulation_id FROM assessment_targets t2 JOIN assessments a2 ON a2.tenant_id=t2.tenant_id AND a2.id=t2.assessment_id WHERE t2.tenant_id=$1 AND a2.organization_id=$2))`, tenant, org).Scan(&crossReference); err != nil {
		return out, err
	}
	if crossReference {
		out.Blockers = append(out.Blockers, "Another organisation retains a reference to this organisation's data. Resolve that reference first; no data will be removed.")
	}
	return out, nil
}
func (s *Service) Preview(ctx context.Context, p *auth.Principal, org uuid.UUID) (Preview, error) {
	if err := deletePermission(p); err != nil {
		return Preview{}, err
	}
	var out Preview
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error { var err error; out, err = preview(ctx, tx, p.TenantID, org, false); return err })
	return out, err
}
func (s *Service) Deletion(ctx context.Context, p *auth.Principal, id uuid.UUID) (Deletion, error) {
	if err := deletePermission(p); err != nil {
		return Deletion{}, err
	}
	var out Deletion
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT id,organization_id,organization_name,counts,created_at,completed_at,(SELECT count(*) FROM data_cleanup_tasks t WHERE t.tenant_id=d.tenant_id AND t.operation_id=d.id) FROM data_deletions d WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&out.ID, &out.OrganizationID, &out.OrganizationName, &raw, &out.CreatedAt, &out.CompletedAt, &out.Pending)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Deletion")
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &out.Counts)
	})
	out.Status = "external_cleanup_pending"
	if out.CompletedAt != nil {
		out.Status = "completed"
	}
	return out, err
}
func (s *Service) DeleteOrganization(ctx context.Context, p *auth.Principal, org uuid.UUID, in DeleteInput, ip, request string) (Deletion, error) {
	if err := deletePermission(p); err != nil {
		return Deletion{}, err
	}
	operation := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		current, err := preview(ctx, tx, p.TenantID, org, true)
		if err != nil {
			return err
		}
		if current.Name != in.Name {
			return httpx.Validation(httpx.FieldError{Name: "name", Message: "Type the organisation name exactly to confirm deletion."})
		}
		if current.Version != in.Version || !reflect.DeepEqual(current.Counts, in.Counts) {
			return httpx.Conflict("Organisation data changed; review a new deletion preview.")
		}
		if len(current.Blockers) > 0 {
			return httpx.Conflict(current.Blockers[0])
		}
		// Claiming reports also locks these rows. A render begun after preview must
		// become visible before any object cleanup intent is committed.
		if _, err = tx.Exec(ctx, `SELECT id FROM reports WHERE tenant_id=$1 AND organization_id=$2 ORDER BY id FOR UPDATE`, p.TenantID, org); err != nil {
			return err
		}
		var rendering bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reports WHERE tenant_id=$1 AND organization_id=$2 AND status='rendering')`, p.TenantID, org).Scan(&rendering); err != nil {
			return err
		}
		if rendering {
			return httpx.Conflict("A PDF is rendering; retry deletion after it finishes.")
		}
		if _, err = tx.Exec(ctx, `UPDATE agents SET status='revoked',revoked_at=COALESCE(revoked_at,now()),revoked_by=$3,updated_at=now() WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2))`, p.TenantID, org, p.UserID); err != nil {
			return err
		}
		if err = removeOrganization(ctx, tx, p.TenantID, org, operation, p.UserID, current); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "organization.delete", ResourceType: "organization", ResourceID: &org, IP: ip, RequestID: request, Detail: map[string]any{"counts": current.Counts, "deletion_id": operation, "external_cleanup": "durable_after_commit"}})
	})
	if err != nil {
		return Deletion{}, err
	}
	return s.Deletion(ctx, p, operation)
}
