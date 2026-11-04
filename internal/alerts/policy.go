package alerts

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/scancontrol"
)

type Policy struct {
	OfflineMinutes int `json:"offline_minutes"`
	Version        int `json:"version"`
}

func inboxAccess(p *auth.Principal) error {
	if p == nil {
		return httpx.Unauthorized()
	}
	for _, perm := range []string{authz.AlertRead, authz.FindingRead, authz.ReportRead} {
		if authz.Has(p.Permissions, perm) {
			return nil
		}
	}
	return httpx.Forbidden(authz.AlertRead)
}
func CurrentPolicy(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (Policy, error) {
	p := Policy{OfflineMinutes: 15}
	err := tx.QueryRow(ctx, `SELECT offline_minutes,version FROM notification_policies WHERE tenant_id=$1`, tenant).Scan(&p.OfflineMinutes, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return p, err
}
func (s *Service) GetPolicy(ctx context.Context, p *auth.Principal) (Policy, error) {
	if !authz.Has(p.Permissions, authz.SettingsManage) {
		return Policy{}, httpx.Forbidden(authz.SettingsManage)
	}
	var out Policy
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error { var e error; out, e = CurrentPolicy(ctx, tx, p.TenantID); return e })
	return out, err
}
func (s *Service) SetPolicy(ctx context.Context, p *auth.Principal, in Policy, ip, req string) (Policy, error) {
	if !authz.Has(p.Permissions, authz.SettingsManage) {
		return Policy{}, httpx.Forbidden(authz.SettingsManage)
	}
	if in.OfflineMinutes < 2 || in.OfflineMinutes > 10080 || in.Version < 0 {
		return Policy{}, httpx.Validation(httpx.FieldError{Name: "offline_minutes", Message: "Choose 2 to 10080 minutes."})
	}
	out := in
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, p.TenantID); e != nil {
			return e
		}
		old, e := CurrentPolicy(ctx, tx, p.TenantID)
		if e != nil {
			return e
		}
		if old.Version != in.Version {
			return httpx.Conflict("Notification policy changed; reload before saving.")
		}
		e = tx.QueryRow(ctx, `INSERT INTO notification_policies(tenant_id,offline_minutes) VALUES($1,$2) ON CONFLICT(tenant_id) DO UPDATE SET offline_minutes=EXCLUDED.offline_minutes,version=notification_policies.version+1,updated_at=now() RETURNING version`, p.TenantID, in.OfflineMinutes).Scan(&out.Version)
		if e != nil {
			return e
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "notifications.policy", ResourceType: "settings", IP: ip, RequestID: req, Detail: map[string]any{"offline_minutes": in.OfflineMinutes, "version": out.Version}})
	})
	return out, err
}
func (h *Handlers) registerPolicy(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/alerts/policy", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		out, e := h.Svc.GetPolicy(r.Context(), p)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, out)
	}))
	m.HandleFunc("PUT /api/v1/alerts/policy", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in Policy
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		out, e := h.Svc.SetPolicy(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, out)
	}))
}
