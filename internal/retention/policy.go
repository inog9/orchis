// Package retention manages bounded lifetimes and durable external deletion intents.
package retention

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"time"
)

const MinimumAuditDays = 90

// Defaults are independent lifetimes: frozen snapshots and old backups have their own policies.
func Defaults() Policy {
	return Policy{FindingsDays: 180, ScanDays: 90, ReportDays: 180, EndpointDays: 30, AuditDays: 365}
}

type Policy struct {
	FindingsDays   int        `json:"findings_days"`
	ScanDays       int        `json:"scan_days"`
	ReportDays     int        `json:"report_days"`
	EndpointDays   int        `json:"endpoint_days"`
	AuditDays      int        `json:"audit_days"`
	Version        int        `json:"version"`
	LastErrorCode  string     `json:"last_error_code,omitempty"`
	LastSweepAt    *time.Time `json:"last_sweep_at,omitempty"`
	CleanupPending int        `json:"cleanup_pending"`
}
type BlobCleaner interface {
	Remove(context.Context, string) error
}
type SearchCleaner interface {
	RemoveFindings(context.Context, uuid.UUID, []uuid.UUID) error
}
type Service struct {
	DB     *db.DB
	Now    func() time.Time
	Blob   BlobCleaner
	Search SearchCleaner
}

func permission(p *auth.Principal, perms ...string) error {
	for _, perm := range append([]string{authz.SettingsManage}, perms...) {
		if !authz.Has(p.Permissions, perm) {
			return httpx.Forbidden(perm)
		}
	}
	return nil
}
func (p Policy) Validate() error {
	for _, v := range []struct {
		name      string
		days, min int
	}{{"findings_days", p.FindingsDays, 1}, {"scan_days", p.ScanDays, 1}, {"report_days", p.ReportDays, 1}, {"endpoint_days", p.EndpointDays, 1}, {"audit_days", p.AuditDays, MinimumAuditDays}} {
		if v.days < v.min || v.days > 3650 {
			return httpx.Validation(httpx.FieldError{Name: v.name, Message: "Retention must be within its allowed day range."})
		}
	}
	if p.Version < 0 {
		return httpx.Validation(httpx.FieldError{Name: "version", Message: "Invalid policy version."})
	}
	return nil
}
func readPolicy(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (Policy, error) {
	out := Defaults()
	err := tx.QueryRow(ctx, `SELECT findings_days,scan_days,report_days,endpoint_days,audit_days,version,last_sweep_at,last_error_code FROM retention_policies WHERE tenant_id=$1`, tenant).Scan(&out.FindingsDays, &out.ScanDays, &out.ReportDays, &out.EndpointDays, &out.AuditDays, &out.Version, &out.LastSweepAt, &out.LastErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM data_cleanup_tasks WHERE tenant_id=$1`, tenant).Scan(&out.CleanupPending)
	return out, err
}
func (s *Service) Get(ctx context.Context, p *auth.Principal) (Policy, error) {
	if err := permission(p); err != nil {
		return Policy{}, err
	}
	var out Policy
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error { var err error; out, err = readPolicy(ctx, tx, p.TenantID); return err })
	return out, err
}
func (s *Service) Update(ctx context.Context, p *auth.Principal, in Policy, ip, request string) (Policy, error) {
	if err := permission(p); err != nil {
		return Policy{}, err
	}
	if err := in.Validate(); err != nil {
		return Policy{}, err
	}
	var out Policy
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "retention:"+p.TenantID.String()); err != nil {
			return err
		}
		current, err := readPolicy(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return httpx.Conflict("Retention policy changed; reload before saving.")
		}
		_, err = tx.Exec(ctx, `INSERT INTO retention_policies(tenant_id,findings_days,scan_days,report_days,endpoint_days,audit_days) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id) DO UPDATE SET findings_days=EXCLUDED.findings_days,scan_days=EXCLUDED.scan_days,report_days=EXCLUDED.report_days,endpoint_days=EXCLUDED.endpoint_days,audit_days=EXCLUDED.audit_days,version=retention_policies.version+1,updated_at=now(),last_sweep_at=NULL`, p.TenantID, in.FindingsDays, in.ScanDays, in.ReportDays, in.EndpointDays, in.AuditDays)
		if err != nil {
			return err
		}
		if err = audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "retention.policy.update", ResourceType: "settings", IP: ip, RequestID: request, Detail: map[string]any{"findings_days": in.FindingsDays, "scan_days": in.ScanDays, "report_days": in.ReportDays, "endpoint_days": in.EndpointDays, "audit_days": in.AuditDays}}); err != nil {
			return err
		}
		out, err = readPolicy(ctx, tx, p.TenantID)
		return err
	})
	return out, err
}
func cutoff(now time.Time, days int) time.Time {
	return now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
}
