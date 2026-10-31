// Package alerts: notifikasi dalam aplikasi (hanya Alerts; tanpa email/SMS). Peristiwa menulis alert dalam transaksi yang sama
// dengan kejadiannya, dengan dedupe_key agar kejadian yang sama tidak menghasilkan alert ganda.
package alerts

import (
	"context"
	"errors"
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
	var dedupe *string
	if a.DedupeKey != "" {
		dedupe = &a.DedupeKey
	}
	_, err := tx.Exec(ctx, `
INSERT INTO alerts (id, tenant_id, kind, severity, title, body, resource_type, resource_id, audience_permission, dedupe_key)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10)
ON CONFLICT (tenant_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`,
		ids.New(), tenant, a.Kind, a.Severity, clip(a.Title, 200), clip(a.Body, 1000), a.ResourceType, a.ResourceID, a.Audience, dedupe)
	return err
}

type Alert struct {
	ID           uuid.UUID  `json:"id"`
	Kind         string     `json:"kind"`
	Severity     string     `json:"severity"`
	Title        string     `json:"title"`
	Body         *string    `json:"body"`
	ResourceType *string    `json:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id"`
	CreatedAt    time.Time  `json:"created_at"`
	Read         bool       `json:"read"`
}

type List struct {
	Items  []Alert `json:"items"`
	Unread int     `json:"unread"`
}

type Service struct{ DB *db.DB }

// visible = alert yang boleh dilihat pengguna ini dan belum ia singkirkan.
const visible = `a.tenant_id=$1 AND a.audience_permission = ANY($2) AND s.dismissed_at IS NULL`
const from = `FROM alerts a LEFT JOIN alert_states s ON s.tenant_id=a.tenant_id AND s.alert_id=a.id AND s.user_id=$3`

func (s *Service) List(ctx context.Context, p *auth.Principal, unreadOnly bool, limit int) (*List, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	out := &List{Items: []Alert{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) `+from+` WHERE `+visible+` AND s.read_at IS NULL`, p.TenantID, p.Permissions, p.UserID).Scan(&out.Unread); err != nil {
			return err
		}
		q := `SELECT a.id, a.kind, a.severity, a.title, a.body, a.resource_type, a.resource_id, a.created_at, s.read_at IS NOT NULL ` + from + ` WHERE ` + visible
		if unreadOnly {
			q += ` AND s.read_at IS NULL`
		}
		rows, err := tx.Query(ctx, q+` ORDER BY a.created_at DESC, a.id DESC LIMIT `+strconv.Itoa(limit), p.TenantID, p.Permissions, p.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a Alert
			if err := rows.Scan(&a.ID, &a.Kind, &a.Severity, &a.Title, &a.Body, &a.ResourceType, &a.ResourceID, &a.CreatedAt, &a.Read); err != nil {
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
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		q := `INSERT INTO alert_states (tenant_id, alert_id, user_id, ` + col + `)
SELECT a.tenant_id, a.id, $3, now() FROM alerts a WHERE a.tenant_id=$1 AND a.audience_permission = ANY($2)`
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
	mux.HandleFunc("GET /api/v1/alerts", auth.Require(authz.AlertRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		l, err := h.Svc.List(r.Context(), p, r.URL.Query().Get("unread") == "1", limit)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, l)
	}))
	one := func(name string, f func(ctx context.Context, p *auth.Principal, id uuid.UUID) error) {
		mux.HandleFunc("POST /api/v1/alerts/{id}/"+name, auth.Require(authz.AlertRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
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
	mux.HandleFunc("POST /api/v1/alerts/read-all", auth.Require(authz.AlertRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if err := h.Svc.ReadAll(r.Context(), p); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

var _ = errors.New
