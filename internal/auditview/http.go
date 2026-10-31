// Package auditview exposes tenant-scoped audit metadata, never raw request payloads.
package auditview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"strconv"
	"time"
)

type Handler struct{ DB *db.DB }
type Entry struct {
	ID           uuid.UUID  `json:"id"`
	At           time.Time  `json:"at"`
	ActorUser    *uuid.UUID `json:"actor_user_id"`
	Actor        string     `json:"actor"`
	Action       string     `json:"action"`
	ResourceType string     `json:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id"`
	IP           string     `json:"ip"`
	RequestID    string     `json:"request_id"`
	Reason       string     `json:"reason"`
}
type Page struct {
	Items      []Entry `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
type cursor struct {
	At time.Time `json:"at"`
	ID uuid.UUID `json:"id"`
}

func (h *Handler) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/audit", auth.Require(authz.AuditRead, h.List))
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "limit", Message: "Choose a page size from 1 to 100."}))
			return
		}
		limit = n
	}
	var after *cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var c cursor
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 1024 || e != nil || json.Unmarshal(b, &c) != nil || c.ID == uuid.Nil || c.At.IsZero() {
			httpx.Write(w, r, httpx.BadRequest("Invalid audit cursor."))
			return
		}
		after = &c
	}
	action := r.URL.Query().Get("action")
	if len(action) > 100 {
		httpx.Write(w, r, httpx.BadRequest("Invalid audit action filter."))
		return
	}
	page, e := h.list(r.Context(), p.TenantID, limit, after, action)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, page)
}
func (h *Handler) list(ctx context.Context, tenant uuid.UUID, limit int, after *cursor, action string) (Page, error) {
	page := Page{Items: []Entry{}}
	var at *time.Time
	var id *uuid.UUID
	if after != nil {
		at = &after.At
		id = &after.ID
	}
	e := h.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT a.id,a.at,a.actor_user_id,COALESCE(u.display_name,''),a.action,COALESCE(a.resource_type,''),a.resource_id,COALESCE(host(a.ip),''),COALESCE(a.request_id,''),
  CASE WHEN a.action IN ('auth.login','auth.logout','auth.login_failed') AND a.detail->>'reason' IN ('authenticated','invalid_credentials','locked') THEN a.detail->>'reason' ELSE '' END
  FROM audit_log a LEFT JOIN users u ON u.tenant_id=a.tenant_id AND u.id=a.actor_user_id
  WHERE a.tenant_id=$1 AND ($2::timestamptz IS NULL OR (a.at,a.id)<($2,$3::uuid)) AND ($4='' OR a.action=$4)
  ORDER BY a.at DESC,a.id DESC LIMIT $5`, tenant, at, id, action, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var item Entry
			if e = rows.Scan(&item.ID, &item.At, &item.ActorUser, &item.Actor, &item.Action, &item.ResourceType, &item.ResourceID, &item.IP, &item.RequestID, &item.Reason); e != nil {
				return e
			}
			page.Items = append(page.Items, item)
		}
		return rows.Err()
	})
	if e != nil {
		return page, e
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		b, _ := json.Marshal(cursor{At: last.At, ID: last.ID})
		value := base64.RawURLEncoding.EncodeToString(b)
		page.NextCursor = &value
	}
	return page, nil
}
