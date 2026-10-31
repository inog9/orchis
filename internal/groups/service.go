package groups

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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

type Group struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Kind        string    `json:"kind"`
	Rule        Rule      `json:"rule"`
	IsSystem    bool      `json:"is_system"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

type Member struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Platform    *string   `json:"platform"`
	Arch        *string   `json:"arch"`
	Criticality string    `json:"criticality"`
}

type Detail struct {
	Group
	Members       []Member `json:"members"`
	TotalAssets   int      `json:"total_assets"`
	MembersCapped bool     `json:"members_capped"`
}

type Service struct{ DB *db.DB }

const (
	memberLimit = 50
	listLimit   = 200
	groupCols   = `g.id, g.name, g.description, g.kind, g.rule, g.is_system, g.created_at, g.updated_at, g.version,
 (SELECT count(*) FROM asset_group_members m WHERE m.tenant_id = g.tenant_id AND m.asset_group_id = g.id)`
)

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	var raw []byte
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.Kind, &raw, &g.IsSystem, &g.CreatedAt, &g.UpdatedAt, &g.Version, &g.MemberCount); err != nil {
		return g, err
	}
	if err := json.Unmarshal(raw, &g.Rule); err != nil {
		return g, err
	}
	if g.Rule.Filters == nil {
		g.Rule.Filters = []Filter{}
	}
	return g, nil
}

// RecomputeAll menghitung ulang keanggotaan semua grup dinamis tenant, dalam transaksi pemanggil.
// Dipanggil setiap kali aset (atau tag/alamatnya) berubah. Pada skala MVP (puluhan grup) ini murah;
// bila membesar, ganti dengan penghitungan ulang berbasis antrean (outbox).
func RecomputeAll(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id, rule FROM asset_groups WHERE tenant_id=$1 AND kind='dynamic'`, tenant)
	if err != nil {
		return err
	}
	type item struct {
		id   uuid.UUID
		rule Rule
	}
	var list []item
	for rows.Next() {
		var it item
		var raw []byte
		if err := rows.Scan(&it.id, &raw); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(raw, &it.rule); err != nil {
			rows.Close()
			return err
		}
		list = append(list, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range list {
		if err := recompute(ctx, tx, tenant, it.id, it.rule); err != nil {
			return err
		}
	}
	return nil
}

func recompute(ctx context.Context, tx pgx.Tx, tenant, group uuid.UUID, rule Rule) error {
	if _, err := tx.Exec(ctx, `DELETE FROM asset_group_members WHERE tenant_id=$1 AND asset_group_id=$2 AND source='rule'`, tenant, group); err != nil {
		return err
	}
	where, args := rule.Where(3)
	_, err := tx.Exec(ctx, `INSERT INTO asset_group_members (tenant_id, asset_group_id, asset_id, source)
SELECT $1, $2, a.id, 'rule' FROM assets a WHERE a.tenant_id = $1 AND a.deleted_at IS NULL AND `+where+` ON CONFLICT DO NOTHING`,
		append([]any{tenant, group}, args...)...)
	return err
}

// SeedSystem membuat grup bawaan (semua endpoint, dan per OS/arsitektur). Aman dipanggil berulang.
func SeedSystem(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	type def struct{ name, platform, arch string }
	defs := []def{{"All endpoints", "", ""}}
	for _, pl := range []struct{ key, label string }{{"linux", "Linux"}, {"macos", "macOS"}, {"windows", "Windows"}} {
		for _, ar := range []string{"arm64", "x86_64"} {
			defs = append(defs, def{"All " + pl.label + " " + ar, pl.key, ar})
		}
	}
	for _, d := range defs {
		r := Rule{Mode: "and", Filters: []Filter{{Field: "platform", Operator: "is_not_empty", Values: []string{}}}}
		if d.platform != "" {
			r.Filters = []Filter{{Field: "platform", Operator: "eq", Values: []string{d.platform}}, {Field: "arch", Operator: "eq", Values: []string{d.arch}}}
		}
		raw, _ := json.Marshal(r)
		id := ids.New()
		var got uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO asset_groups (id, tenant_id, name, kind, rule, is_system) VALUES ($1,$2,$3,'dynamic',$4,true)
ON CONFLICT (tenant_id, name) DO UPDATE SET rule = EXCLUDED.rule WHERE asset_groups.is_system RETURNING id`, id, tenant, d.name, raw).Scan(&got)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // ada grup buatan pengguna dengan nama yang sama; jangan ditimpa
		}
		if err != nil {
			return err
		}
		if err := recompute(ctx, tx, tenant, got, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context, p *auth.Principal) ([]Group, error) {
	out := []Group{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+groupCols+` FROM asset_groups g WHERE g.tenant_id=$1 ORDER BY g.is_system DESC, lower(g.name) LIMIT $2`, p.TenantID, listLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			g, err := scanGroup(rows)
			if err != nil {
				return err
			}
			out = append(out, g)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Detail, error) {
	var d *Detail
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		g, err := scanGroup(tx.QueryRow(ctx, `SELECT `+groupCols+` FROM asset_groups g WHERE g.tenant_id=$1 AND g.id=$2`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset group")
		}
		if err != nil {
			return err
		}
		d = &Detail{Group: g, Members: []Member{}}
		rows, err := tx.Query(ctx, `SELECT a.id, a.name, a.category, a.platform, a.arch, a.criticality
FROM asset_group_members m JOIN assets a ON a.tenant_id = m.tenant_id AND a.id = m.asset_id
WHERE m.tenant_id=$1 AND m.asset_group_id=$2 AND a.deleted_at IS NULL ORDER BY lower(a.name), a.id LIMIT $3`, p.TenantID, id, memberLimit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m Member
			if err := rows.Scan(&m.ID, &m.Name, &m.Category, &m.Platform, &m.Arch, &m.Criticality); err != nil {
				return err
			}
			d.Members = append(d.Members, m)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if len(d.Members) > memberLimit {
			d.Members, d.MembersCapped = d.Members[:memberLimit], true
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND deleted_at IS NULL`, p.TenantID).Scan(&d.TotalAssets)
	})
	return d, err
}

type Input struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Rule        Rule    `json:"rule"`
	Version     *int    `json:"version"`
}

func (in *Input) validate() error {
	var fe []httpx.FieldError
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		fe = append(fe, httpx.FieldError{Name: "name", Message: "Name is required (max 100 characters)."})
	}
	if in.Description != nil && len(*in.Description) > 1000 {
		fe = append(fe, httpx.FieldError{Name: "description", Message: "Description is too long (max 1000 characters)."})
	}
	if len(fe) > 0 {
		return httpx.Validation(fe...)
	}
	return in.Rule.Normalize()
}

func nameTaken(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return httpx.Validation(httpx.FieldError{Name: "name", Message: "Another group already uses this name."})
	}
	return err
}

func (s *Service) Create(ctx context.Context, p *auth.Principal, in Input, ip, reqID string) (*Detail, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	id := ids.New()
	rule, _ := json.Marshal(in.Rule)
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO asset_groups (id, tenant_id, name, description, kind, rule) VALUES ($1,$2,$3,$4,'dynamic',$5)`,
			id, p.TenantID, in.Name, in.Description, rule); err != nil {
			return err
		}
		if err := recompute(ctx, tx, p.TenantID, id, in.Rule); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset_group.create", ResourceType: "asset_group",
			ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": in.Name, "filters": len(in.Rule.Filters)}})
	})
	if err != nil {
		return nil, nameTaken(err)
	}
	return s.Get(ctx, p, id)
}

func (s *Service) Update(ctx context.Context, p *auth.Principal, id uuid.UUID, in Input, ip, reqID string) (*Detail, error) {
	if in.Version == nil {
		return nil, httpx.Validation(httpx.FieldError{Name: "version", Message: "The version you are editing is required."})
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	rule, _ := json.Marshal(in.Rule)
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var system bool
		var cur int
		err := tx.QueryRow(ctx, `SELECT is_system, version FROM asset_groups WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id).Scan(&system, &cur)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset group")
		}
		if err != nil {
			return err
		}
		if system {
			return httpx.Conflict("Built-in groups cannot be edited.")
		}
		if cur != *in.Version {
			return httpx.Conflict("This group was changed by someone else. Reload and try again.")
		}
		if _, err := tx.Exec(ctx, `UPDATE asset_groups SET name=$3, description=$4, rule=$5, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			p.TenantID, id, in.Name, in.Description, rule); err != nil {
			return err
		}
		if err := recompute(ctx, tx, p.TenantID, id, in.Rule); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset_group.update", ResourceType: "asset_group",
			ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": in.Name}})
	})
	if err != nil {
		return nil, nameTaken(err)
	}
	return s.Get(ctx, p, id)
}

func (s *Service) Delete(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var system bool
		var name string
		err := tx.QueryRow(ctx, `SELECT is_system, name FROM asset_groups WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id).Scan(&system, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset group")
		}
		if err != nil {
			return err
		}
		if system {
			return httpx.Conflict("Built-in groups cannot be deleted.")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM asset_groups WHERE tenant_id=$1 AND id=$2`, p.TenantID, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset_group.delete", ResourceType: "asset_group",
			ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": name}})
	})
}

type Preview struct {
	Count  int      `json:"count"`
	Total  int      `json:"total"`
	Sample []Member `json:"sample"`
}

// Preview menghitung siapa yang akan cocok TANPA menyimpan apa pun.
func (s *Service) Preview(ctx context.Context, p *auth.Principal, rule Rule) (*Preview, error) {
	if err := rule.Normalize(); err != nil {
		return nil, err
	}
	out := &Preview{Sample: []Member{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		where, args := rule.Where(2)
		base := append([]any{p.TenantID}, args...)
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets a WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND `+where, base...).Scan(&out.Count); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND deleted_at IS NULL`, p.TenantID).Scan(&out.Total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT a.id, a.name, a.category, a.platform, a.arch, a.criticality FROM assets a
WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND `+where+` ORDER BY lower(a.name), a.id LIMIT 10`, base...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m Member
			if err := rows.Scan(&m.ID, &m.Name, &m.Category, &m.Platform, &m.Arch, &m.Criticality); err != nil {
				return err
			}
			out.Sample = append(out.Sample, m)
		}
		return rows.Err()
	})
	return out, err
}

// ---- HTTP ----

type Handlers struct{ Svc *Service }

func (h *Handlers) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	list, err := h.Svc.List(r.Context(), p)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list, "next_cursor": nil})
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Asset group"))
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := h.Svc.Get(r.Context(), p, id)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	d, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/asset-groups/"+d.ID.String())
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	d, err := h.Svc.Update(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.Svc.Delete(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Preview(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in struct {
		Rule Rule `json:"rule"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	pv, err := h.Svc.Preview(r.Context(), p, in.Rule)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pv)
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/asset-groups", auth.Require(authz.AssetRead, h.List))
	mux.HandleFunc("POST /api/v1/asset-groups/preview", auth.Require(authz.AssetRead, h.Preview))
	mux.HandleFunc("GET /api/v1/asset-groups/{id}", auth.Require(authz.AssetRead, h.Get))
	mux.HandleFunc("POST /api/v1/asset-groups", auth.Require(authz.AssetWrite, h.Create))
	mux.HandleFunc("PUT /api/v1/asset-groups/{id}", auth.Require(authz.AssetWrite, h.Update))
	mux.HandleFunc("DELETE /api/v1/asset-groups/{id}", auth.Require(authz.AssetDelete, h.Delete))
}
