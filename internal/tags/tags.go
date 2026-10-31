// Package tags: label berwarna untuk aset (dan nanti scenario).
package tags

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

type Tag struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Color  string    `json:"color"`
	Assets int       `json:"assets"` // jumlah aset yang memakai tag ini
}

type Service struct{ DB *db.DB }

var colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

const DefaultColor = "#8C919A"

func (s *Service) List(ctx context.Context, p *auth.Principal) ([]Tag, error) {
	out := []Tag{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT t.id, t.name, t.color, (SELECT count(*) FROM asset_tags a WHERE a.tenant_id=t.tenant_id AND a.tag_id=t.id) FROM tags t WHERE t.tenant_id=$1 ORDER BY lower(t.name) LIMIT 500`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t Tag
			if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Assets); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

func Normalize(name, color string) (string, string, error) {
	name = strings.TrimSpace(name)
	var fe []httpx.FieldError
	if name == "" || len(name) > 50 {
		fe = append(fe, httpx.FieldError{Name: "name", Message: "Name is required (max 50 characters)."})
	}
	if color == "" {
		color = DefaultColor
	}
	if !colorRe.MatchString(color) {
		fe = append(fe, httpx.FieldError{Name: "color", Message: "Color must look like #RRGGBB."})
	}
	if len(fe) > 0 {
		return "", "", httpx.Validation(fe...)
	}
	return name, strings.ToUpper(color), nil
}

func (s *Service) Create(ctx context.Context, p *auth.Principal, name, color, ip, reqID string) (*Tag, error) {
	name, color, err := Normalize(name, color)
	if err != nil {
		return nil, err
	}
	t := Tag{ID: ids.New(), Name: name, Color: color}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name, color) VALUES ($1,$2,$3,$4)`, t.ID, p.TenantID, t.Name, t.Color); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "tag.create", ResourceType: "tag",
			ResourceID: &t.ID, IP: ip, RequestID: reqID, Detail: map[string]any{"name": t.Name}})
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, httpx.Conflict("A tag named \"" + name + "\" already exists.")
	}
	return &t, err
}

func (s *Service) Update(ctx context.Context, p *auth.Principal, id uuid.UUID, name, color, ip, reqID string) (*Tag, error) {
	name, color, err := Normalize(name, color)
	if err != nil {
		return nil, err
	}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tags SET name=$3, color=$4, updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, name, color)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Tag")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "tag.update", ResourceType: "tag", ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": name}})
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, httpx.Validation(httpx.FieldError{Name: "name", Message: "A tag named \"" + name + "\" already exists."})
	}
	if err != nil {
		return nil, err
	}
	all, err := s.List(ctx, p)
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, httpx.NotFound("Tag")
}

// Delete menolak tag yang masih dipakai aturan grup (aturan akan menjadi yatim), lalu melepas tag dari aset.
func (s *Service) Delete(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var inRule int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM asset_groups WHERE tenant_id=$1 AND position($2 in rule::text) > 0`, p.TenantID, id.String()).Scan(&inRule); err != nil {
			return err
		}
		if inRule > 0 {
			return httpx.Conflict("This tag is used by an asset group rule. Change the rule first.")
		}
		tag, err := tx.Exec(ctx, `DELETE FROM tags WHERE tenant_id=$1 AND id=$2`, p.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Tag")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "tag.delete", ResourceType: "tag", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}

type Handlers struct{ Svc *Service }

func (h *Handlers) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	list, err := h.Svc.List(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	t, err := h.Svc.Create(r.Context(), p, in.Name, in.Color, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/tags", auth.Require(authz.AssetRead, h.List))
	mux.HandleFunc("POST /api/v1/tags", auth.Require(authz.AssetWrite, h.Create))
	mux.HandleFunc("PATCH /api/v1/tags/{id}", auth.Require(authz.AssetWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Tag"))
			return
		}
		var in struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		t, err := h.Svc.Update(r.Context(), p, id, in.Name, in.Color, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, t)
	}))
	mux.HandleFunc("DELETE /api/v1/tags/{id}", auth.Require(authz.AssetDelete, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Tag"))
			return
		}
		if err := h.Svc.Delete(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
