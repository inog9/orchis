// Package alerts: notifikasi dalam aplikasi (hanya Alerts; tanpa email/SMS). Peristiwa menulis alert dalam transaksi yang sama
// dengan kejadiannya, dengan dedupe_key agar kejadian yang sama tidak menghasilkan alert ganda.
package alerts

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

// New menggambarkan alert yang akan dibuat.
type New struct {
	Kind         string
	Severity     string // info | warning | critical
	Title        string
	Body         string
	ResourceType string
	ResourceID   *uuid.UUID
	Audience     string // permission yang berhak melihat; kosong = alert.read
	DedupeKey    string
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Raise menulis alert dalam transaksi pemanggil. Alert dengan dedupe_key yang sudah ada diabaikan.
func Raise(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a New) error {
	if a.Severity == "" {
		a.Severity = "info"
	}
	if a.Audience == "" {
		a.Audience = authz.AlertRead
	}
	var org *uuid.UUID
	if a.ResourceID != nil {
		if err := tx.QueryRow(ctx, `SELECT CASE $2
 WHEN 'audit_schedule' THEN (SELECT organization_id FROM audit_schedules WHERE tenant_id=$1 AND id=$3)
 WHEN 'organization' THEN (SELECT id FROM organizations WHERE tenant_id=$1 AND id=$3)
 WHEN 'agent' THEN (SELECT organization_id FROM agents WHERE tenant_id=$1 AND id=$3)
 WHEN 'asset' THEN (SELECT organization_id FROM assets WHERE tenant_id=$1 AND id=$3 AND deleted_at IS NULL)
 WHEN 'finding' THEN (SELECT x.organization_id FROM findings f JOIN assets x ON x.tenant_id=f.tenant_id AND x.id=f.asset_id WHERE f.tenant_id=$1 AND f.id=$3)
 WHEN 'scan' THEN (SELECT organization_id FROM assessments WHERE tenant_id=$1 AND id=$3 AND is_scan)
 WHEN 'report' THEN (SELECT organization_id FROM reports WHERE tenant_id=$1 AND id=$3)
 END`, tenant, a.ResourceType, *a.ResourceID).Scan(&org); err != nil {
			return err
		}
	}
	var dedupe *string
	if a.DedupeKey != "" {
		dedupe = &a.DedupeKey
	}
	_, err := tx.Exec(ctx, `
INSERT INTO alerts (id, tenant_id, kind, severity, title, body, resource_type, resource_id, audience_permission, dedupe_key,organization_id)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11)
ON CONFLICT (tenant_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`,
		ids.New(), tenant, a.Kind, a.Severity, clip(a.Title, 200), clip(a.Body, 1000), a.ResourceType, a.ResourceID, a.Audience, dedupe, org)
	return err
}

type Alert struct {
	ID             uuid.UUID  `json:"id"`
	Kind           string     `json:"kind"`
	Severity       string     `json:"severity"`
	Title          string     `json:"title"`
	Body           *string    `json:"body"`
	ResourceType   *string    `json:"resource_type"`
	ResourceID     *uuid.UUID `json:"resource_id"`
	OrganizationID *uuid.UUID `json:"organization_id"`
	ResourceLabel  *string    `json:"resource_label"`
	CreatedAt      time.Time  `json:"created_at"`
	Read           bool       `json:"read"`
}

type List struct {
	Items  []Alert `json:"items"`
	Unread int     `json:"unread"`
}

type Service struct{ DB *db.DB }

// visible = alert yang boleh dilihat pengguna ini dan belum ia singkirkan.
// Object visibility is checked again for list, counts and every marking operation.
// Roles grant tenant-wide organization access; there is no separate user/org ACL.
const objectVisible = `a.tenant_id=$1 AND a.audience_permission = ANY($2) AND
 (a.organization_id IS NULL OR EXISTS(SELECT 1 FROM organizations o WHERE o.tenant_id=a.tenant_id AND o.id=a.organization_id)) AND
 CASE a.resource_type
 WHEN 'audit_schedule' THEN 'assessment.read'=ANY($2) AND 'simulation.read'=ANY($2) AND 'organization.read'=ANY($2) AND 'asset.read'=ANY($2) AND 'finding.read'=ANY($2) AND 'report.read'=ANY($2) AND EXISTS(SELECT 1 FROM audit_schedules x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id AND x.deleted_at IS NULL AND x.organization_id=a.organization_id)
 WHEN 'report' THEN 'report.read'=ANY($2) AND 'finding.read'=ANY($2) AND EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.id=a.resource_id AND r.organization_id IS NOT DISTINCT FROM a.organization_id)
 WHEN 'scan' THEN 'assessment.read'=ANY($2) AND 'organization.read'=ANY($2) AND 'asset.read'=ANY($2) AND EXISTS(SELECT 1 FROM assessments x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id AND x.is_scan AND x.organization_id IS NOT DISTINCT FROM a.organization_id)
 WHEN 'agent' THEN 'agent.read'=ANY($2) AND 'asset.read'=ANY($2) AND (a.organization_id IS NULL OR 'organization.read'=ANY($2)) AND EXISTS(SELECT 1 FROM agents g JOIN assets x ON x.tenant_id=g.tenant_id AND x.id=g.asset_id AND x.deleted_at IS NULL AND x.organization_id IS NOT DISTINCT FROM g.organization_id WHERE g.tenant_id=a.tenant_id AND g.id=a.resource_id AND g.organization_id IS NOT DISTINCT FROM a.organization_id)
 WHEN 'asset' THEN 'asset.read'=ANY($2) AND (a.organization_id IS NULL OR 'organization.read'=ANY($2)) AND EXISTS(SELECT 1 FROM assets x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id AND x.deleted_at IS NULL AND x.organization_id IS NOT DISTINCT FROM a.organization_id)
 WHEN 'organization' THEN 'organization.read'=ANY($2) AND EXISTS(SELECT 1 FROM organizations o WHERE o.tenant_id=a.tenant_id AND o.id=a.resource_id AND o.id=a.organization_id)
 WHEN 'finding' THEN 'finding.read'=ANY($2) AND EXISTS(SELECT 1 FROM findings f LEFT JOIN assets x ON x.tenant_id=f.tenant_id AND x.id=f.asset_id WHERE f.tenant_id=a.tenant_id AND f.id=a.resource_id AND x.organization_id IS NOT DISTINCT FROM a.organization_id)
 WHEN 'simulation' THEN 'simulation.read'=ANY($2) AND 'asset.read'=ANY($2) AND EXISTS(SELECT 1 FROM simulations x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id)
 WHEN 'execution' THEN 'atomic.read'=ANY($2) AND 'asset.read'=ANY($2) AND EXISTS(SELECT 1 FROM executions x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id)
 WHEN 'user' THEN 'user.manage'=ANY($2) AND EXISTS(SELECT 1 FROM users x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id)
 ELSE a.resource_type IS NULL END`

// Presentation labels are selected from the same authorized object, never from a supplied URL.
const resourceLabel = `left(CASE a.resource_type
 WHEN 'audit_schedule' THEN (SELECT x.name FROM audit_schedules x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id AND x.deleted_at IS NULL)
 WHEN 'report' THEN (SELECT r.snapshot->>'organization' FROM reports r WHERE r.tenant_id=a.tenant_id AND r.id=a.resource_id)
 WHEN 'scan' THEN (SELECT x.name FROM assessments x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id)
 WHEN 'agent' THEN (SELECT COALESCE(NULLIF(g.instance_name,''),g.hostname) FROM agents g WHERE g.tenant_id=a.tenant_id AND g.id=a.resource_id)
 WHEN 'asset' THEN (SELECT x.name FROM assets x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id)
 WHEN 'organization' THEN (SELECT o.name FROM organizations o WHERE o.tenant_id=a.tenant_id AND o.id=a.resource_id)
 WHEN 'finding' THEN (SELECT f.title FROM findings f WHERE f.tenant_id=a.tenant_id AND f.id=a.resource_id)
 WHEN 'simulation' THEN (SELECT x.name FROM simulations x WHERE x.tenant_id=a.tenant_id AND x.id=a.resource_id)
 WHEN 'user' THEN (SELECT u.display_name FROM users u WHERE u.tenant_id=a.tenant_id AND u.id=a.resource_id)
 END,200)`
const visible = objectVisible + ` AND s.dismissed_at IS NULL`
const from = `FROM alerts a LEFT JOIN alert_states s ON s.tenant_id=a.tenant_id AND s.alert_id=a.id AND s.user_id=$3`

func (s *Service) List(ctx context.Context, p *auth.Principal, unreadOnly bool, limit int) (*List, error) {
	return s.ListOrganization(ctx, p, unreadOnly, limit, nil)
}
func (s *Service) ListOrganization(ctx context.Context, p *auth.Principal, unreadOnly bool, limit int, org *uuid.UUID) (*List, error) {
	if err := inboxAccess(p); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	out := &List{Items: []Alert{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) `+from+` WHERE `+visible+` AND s.read_at IS NULL AND ($4::uuid IS NULL OR a.organization_id=$4)`, p.TenantID, p.Permissions, p.UserID, org).Scan(&out.Unread); err != nil {
			return err
		}
		q := `SELECT a.id, a.kind, a.severity, a.title, a.body, a.resource_type, a.resource_id, a.organization_id, ` + resourceLabel + `, a.created_at, s.read_at IS NOT NULL ` + from + ` WHERE ` + visible + ` AND ($4::uuid IS NULL OR a.organization_id=$4)`
		if unreadOnly {
			q += ` AND s.read_at IS NULL`
		}
		rows, err := tx.Query(ctx, q+` ORDER BY a.created_at DESC, a.id DESC LIMIT `+strconv.Itoa(limit), p.TenantID, p.Permissions, p.UserID, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a Alert
			if err := rows.Scan(&a.ID, &a.Kind, &a.Severity, &a.Title, &a.Body, &a.ResourceType, &a.ResourceID, &a.OrganizationID, &a.ResourceLabel, &a.CreatedAt, &a.Read); err != nil {
				return err
			}
			out.Items = append(out.Items, a)
		}
		return rows.Err()
	})
	return out, err
}

// mark memberi tanda baca/singkir untuk alert yang memang boleh dilihat pengguna (alert milik audiens lain dianggap tidak ada).
func (s *Service) mark(ctx context.Context, p *auth.Principal, id *uuid.UUID, col string) error {
	if err := inboxAccess(p); err != nil {
		return err
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		q := `INSERT INTO alert_states (tenant_id, alert_id, user_id, ` + col + `)
SELECT a.tenant_id, a.id, $3, now() FROM alerts a WHERE ` + objectVisible
		args := []any{p.TenantID, p.Permissions, p.UserID}
		if id != nil {
			q += ` AND a.id=$4`
			args = append(args, *id)
		}
		q += ` ON CONFLICT (tenant_id, alert_id, user_id) DO UPDATE SET ` + col + ` = COALESCE(alert_states.` + col + `, now())`
		tag, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return err
		}
		if id != nil && tag.RowsAffected() == 0 {
			return httpx.NotFound("Alert")
		}
		return nil
	})
}

func (s *Service) Read(ctx context.Context, p *auth.Principal, id uuid.UUID) error {
	return s.mark(ctx, p, &id, "read_at")
}
func (s *Service) ReadAll(ctx context.Context, p *auth.Principal) error {
	return s.mark(ctx, p, nil, "read_at")
}
func (s *Service) Dismiss(ctx context.Context, p *auth.Principal, id uuid.UUID) error {
	return s.mark(ctx, p, &id, "dismissed_at")
}

type Handlers struct{ Svc *Service }

func (h *Handlers) Register(mux *http.ServeMux) {
	h.registerPolicy(mux)
	mux.HandleFunc("GET /api/v1/alerts", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		var org *uuid.UUID
		if raw := r.URL.Query().Get("organization_id"); raw != "" {
			id, e := uuid.Parse(raw)
			if e != nil {
				httpx.Write(w, r, httpx.BadRequest("Invalid organization filter."))
				return
			}
			org = &id
		}
		l, err := h.Svc.ListOrganization(r.Context(), p, r.URL.Query().Get("unread") == "1", limit, org)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, l)
	}))
	one := func(name string, f func(ctx context.Context, p *auth.Principal, id uuid.UUID) error) {
		mux.HandleFunc("POST /api/v1/alerts/{id}/"+name, auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
			id, err := uuid.Parse(r.PathValue("id"))
			if err != nil {
				httpx.Write(w, r, httpx.NotFound("Alert"))
				return
			}
			if err := f(r.Context(), p, id); err != nil {
				httpx.Write(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	one("read", h.Svc.Read)
	one("dismiss", h.Svc.Dismiss)
	mux.HandleFunc("POST /api/v1/alerts/read-all", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if err := h.Svc.ReadAll(r.Context(), p); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
