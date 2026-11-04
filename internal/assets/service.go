package assets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/groups"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

type Service struct{ DB *db.DB }

const (
	DefaultLimit = 25
	MaxLimit     = 100
	cols         = `a.id, a.category, a.name, a.description, a.platform, a.arch, a.criticality, a.is_internet_facing,
a.testing_authorized, a.source, a.attributes, a.created_at, a.updated_at, a.version,
 COALESCE((SELECT CASE WHEN g.last_heartbeat_at > now() - interval '90 seconds' THEN 'online' ELSE 'offline' END FROM agents g
  WHERE g.tenant_id=a.tenant_id AND g.asset_id=a.id AND g.status<>'revoked' ORDER BY g.last_heartbeat_at DESC NULLS LAST LIMIT 1), 'none'), a.organization_id,
 (SELECT o.name FROM organizations o WHERE o.tenant_id=a.tenant_id AND o.id=a.organization_id)`
)

type ListFilter struct {
	Organization string
	Category     string
	Query        string
	Limit        int
	Cursor       string
}

type Page struct {
	Items      []Asset `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type cursor struct {
	U time.Time `json:"u"`
	I uuid.UUID `json:"i"`
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, httpx.BadRequest("invalid cursor")
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil || c.I == uuid.Nil {
		return nil, httpx.BadRequest("invalid cursor")
	}
	return &c, nil
}

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

func scanAsset(row pgx.Row) (Asset, error) {
	var a Asset
	err := row.Scan(&a.ID, &a.Category, &a.Name, &a.Description, &a.Platform, &a.Arch, &a.Criticality, &a.IsInternetFacing,
		&a.TestingAuthorized, &a.Source, &a.Attributes, &a.CreatedAt, &a.UpdatedAt, &a.Version, &a.AgentState, &a.OrganizationID, &a.OrganizationName)
	a.Addresses = []Address{}
	return a, err
}

// loadAddresses mengisi alamat untuk banyak aset sekaligus (satu kueri, tanpa N+1).
func loadAddresses(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, list []Asset) error {
	if len(list) == 0 {
		return nil
	}
	idx := map[uuid.UUID]int{}
	idList := make([]uuid.UUID, len(list))
	for i, a := range list {
		idx[a.ID] = i
		idList[i] = a.ID
	}
	rows, err := tx.Query(ctx, `SELECT asset_id, kind, value, is_primary, scan_approved FROM asset_addresses
WHERE tenant_id=$1 AND asset_id = ANY($2) ORDER BY is_primary DESC, kind, value`, tenant, idList)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var ad Address
		if err := rows.Scan(&id, &ad.Kind, &ad.Value, &ad.IsPrimary, &ad.ScanApproved); err != nil {
			return err
		}
		list[idx[id]].Addresses = append(list[idx[id]].Addresses, ad)
	}
	return rows.Err()
}

func (s *Service) List(ctx context.Context, p *auth.Principal, f ListFilter) (*Page, error) {
	if f.Limit <= 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit > MaxLimit {
		f.Limit = MaxLimit
	}
	if f.Category != "" && !oneOf(f.Category, Categories) {
		return nil, httpx.Validation(httpx.FieldError{Name: "category", Message: "Unknown category."})
	}
	if f.Organization != "" && f.Organization != "unassigned" {
		if _, e := uuid.Parse(f.Organization); e != nil {
			return nil, httpx.BadRequest("Invalid organization.")
		}
	}
	cur, err := decodeCursor(f.Cursor)
	if err != nil {
		return nil, err
	}
	var curU *time.Time
	var curI *uuid.UUID
	if cur != nil {
		curU, curI = &cur.U, &cur.I
	}
	var q any
	if f.Query != "" {
		q = likePattern(f.Query)
	}
	var cat any
	if f.Category != "" {
		cat = f.Category
	}
	page := &Page{Items: []Asset{}}
	err = s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM assets a
WHERE a.tenant_id=$1 AND a.deleted_at IS NULL
  AND ($2::text IS NULL OR a.category=$2)
  AND ($3::text IS NULL OR a.name ILIKE $3 ESCAPE '\')
  AND ($7::text = '' OR ($7='unassigned' AND a.organization_id IS NULL) OR a.organization_id::text=$7)
  AND ($4::timestamptz IS NULL OR (a.updated_at, a.id) < ($4, $5::uuid))
ORDER BY a.updated_at DESC, a.id DESC LIMIT $6`, p.TenantID, cat, q, curU, curI, f.Limit+1, f.Organization)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAsset(rows)
			if err != nil {
				return err
			}
			page.Items = append(page.Items, a)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if len(page.Items) > f.Limit {
			page.Items = page.Items[:f.Limit]
			last := page.Items[f.Limit-1]
			c := encodeCursor(cursor{U: last.UpdatedAt, I: last.ID})
			page.NextCursor = &c
		}
		if err := loadAddresses(ctx, tx, p.TenantID, page.Items); err != nil {
			return err
		}
		return loadTags(ctx, tx, p.TenantID, page.Items)
	})
	return page, err
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Asset, error) {
	var out *Asset
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		a, err := scanAsset(tx.QueryRow(ctx, `SELECT `+cols+` FROM assets a WHERE a.tenant_id=$1 AND a.id=$2 AND a.deleted_at IS NULL`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset")
		}
		if err != nil {
			return err
		}
		list := []Asset{a}
		if err := loadAddresses(ctx, tx, p.TenantID, list); err != nil {
			return err
		}
		if err := loadTags(ctx, tx, p.TenantID, list); err != nil {
			return err
		}
		if err := loadGroups(ctx, tx, p.TenantID, &list[0]); err != nil {
			return err
		}
		out = &list[0]
		return nil
	})
	return out, err
}

func (s *Service) Create(ctx context.Context, p *auth.Principal, in CreateInput, ip, requestID string) (*Asset, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	// Menandai aset boleh diuji adalah keputusan keamanan terpisah dari hak menulis aset.
	if in.TestingAuthorized && !authz.Has(p.Permissions, authz.AssetAuthorize) {
		return nil, httpx.Forbidden(authz.AssetAuthorize)
	}
	if in.OrganizationID != nil && !authz.Has(p.Permissions, authz.OrganizationWrite) {
		return nil, httpx.Forbidden(authz.OrganizationWrite)
	}
	id := ids.New()
	var out *Asset
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if in.OrganizationID != nil {
			var demo bool
			err := tx.QueryRow(ctx, `SELECT is_demo FROM organizations WHERE tenant_id=$1 AND id=$2 FOR KEY SHARE`, p.TenantID, *in.OrganizationID).Scan(&demo)
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound("Organization")
			}
			if err != nil {
				return err
			}
			if demo {
				return httpx.Conflict("Demo data is read-only; choose another organisation.")
			}
		}
		var authBy *uuid.UUID
		var authAt *time.Time
		if in.TestingAuthorized {
			now := time.Now()
			authBy, authAt = &p.UserID, &now
		}
		_, err := tx.Exec(ctx, `
INSERT INTO assets (id, tenant_id, category, name, description, platform, arch, criticality, is_internet_facing,
  testing_authorized, authorized_by, authorized_at, source, attributes, created_by,organization_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'manual',$13,$14,$15)`,
			id, p.TenantID, in.Category, in.Name, in.Description, in.Platform, in.Arch, in.Criticality, in.IsInternetFacing,
			in.TestingAuthorized, authBy, authAt, []byte(in.Attributes), p.UserID, in.OrganizationID)
		if err != nil {
			return err
		}
		for i, ad := range in.Addresses {
			if _, err := tx.Exec(ctx, `INSERT INTO asset_addresses (id, tenant_id, asset_id, kind, value, is_primary) VALUES ($1,$2,$3,$4,$5,$6)`,
				ids.New(), p.TenantID, id, ad.Kind, ad.Value, ad.IsPrimary || i == 0); err != nil {
				return err
			}
		}
		if err := setTags(ctx, tx, p.TenantID, id, in.TagIDs); err != nil {
			return err
		}
		if err := groups.RecomputeAll(ctx, tx, p.TenantID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset.create",
			ResourceType: "asset", ResourceID: &id, IP: ip, RequestID: requestID,
			Detail: map[string]any{"name": in.Name, "category": in.Category, "testing_authorized": in.TestingAuthorized, "organization_id": in.OrganizationID}}); err != nil {
			return err
		}
		a, err := scanAsset(tx.QueryRow(ctx, `SELECT `+cols+` FROM assets a WHERE a.tenant_id=$1 AND a.id=$2`, p.TenantID, id))
		if err != nil {
			return err
		}
		list := []Asset{a}
		if err := loadAddresses(ctx, tx, p.TenantID, list); err != nil {
			return err
		}
		if err := loadTags(ctx, tx, p.TenantID, list); err != nil {
			return err
		}
		out = &list[0]
		return nil
	})
	return out, err
}

func loadTags(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, list []Asset) error {
	idx := map[uuid.UUID]int{}
	idList := make([]uuid.UUID, len(list))
	for i := range list {
		list[i].Tags = []TagRef{}
		idx[list[i].ID] = i
		idList[i] = list[i].ID
	}
	if len(list) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT at.asset_id, t.id, t.name, t.color FROM asset_tags at
JOIN tags t ON t.tenant_id = at.tenant_id AND t.id = at.tag_id
WHERE at.tenant_id=$1 AND at.asset_id = ANY($2) ORDER BY lower(t.name)`, tenant, idList)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var aid uuid.UUID
		var t TagRef
		if err := rows.Scan(&aid, &t.ID, &t.Name, &t.Color); err != nil {
			return err
		}
		list[idx[aid]].Tags = append(list[idx[aid]].Tags, t)
	}
	return rows.Err()
}

func loadGroups(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a *Asset) error {
	a.Groups = []GroupRef{}
	rows, err := tx.Query(ctx, `SELECT g.id, g.name, g.is_system FROM asset_group_members m
JOIN asset_groups g ON g.tenant_id = m.tenant_id AND g.id = m.asset_group_id
WHERE m.tenant_id=$1 AND m.asset_id=$2 ORDER BY g.is_system, lower(g.name)`, tenant, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var g GroupRef
		if err := rows.Scan(&g.ID, &g.Name, &g.IsSystem); err != nil {
			return err
		}
		a.Groups = append(a.Groups, g)
	}
	return rows.Err()
}

// setTags mengganti himpunan tag aset. Tag yang tidak ada (atau milik tenant lain) ditolak dengan 422.
func setTags(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, tagIDs []uuid.UUID) error {
	uniq := map[uuid.UUID]bool{}
	var list []uuid.UUID
	for _, id := range tagIDs {
		if !uniq[id] {
			uniq[id] = true
			list = append(list, id)
		}
	}
	if len(list) > 0 {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags WHERE tenant_id=$1 AND id = ANY($2)`, tenant, list).Scan(&n); err != nil {
			return err
		}
		if n != len(list) {
			return httpx.Validation(httpx.FieldError{Name: "tag_ids", Message: "One or more tags do not exist."})
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM asset_tags WHERE tenant_id=$1 AND asset_id=$2`, tenant, asset); err != nil {
		return err
	}
	for _, id := range list {
		if _, err := tx.Exec(ctx, `INSERT INTO asset_tags (tenant_id, asset_id, tag_id) VALUES ($1,$2,$3)`, tenant, asset, id); err != nil {
			return err
		}
	}
	return nil
}

// Patch mengubah sebagian kolom aset. Versi wajib cocok; bila tidak, 409 (ada yang mengubah lebih dulu).
func (s *Service) Patch(ctx context.Context, p *auth.Principal, id uuid.UUID, in PatchInput, ip, reqID string) (*Asset, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	changed := []string{}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var cur int
		var authorized bool
		err := tx.QueryRow(ctx, `SELECT version, testing_authorized FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`,
			p.TenantID, id).Scan(&cur, &authorized)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset")
		}
		if err != nil {
			return err
		}
		if cur != *in.Version {
			return httpx.Conflict("This asset was changed by someone else. Reload and try again.")
		}
		if in.Addresses != nil && authorized && !authz.Has(p.Permissions, authz.AssetAuthorize) {
			return httpx.Forbidden(authz.AssetAuthorize)
		}
		var setAuth *bool
		if in.TestingAuthorized != nil && *in.TestingAuthorized != authorized {
			if !authz.Has(p.Permissions, authz.AssetAuthorize) {
				return httpx.Forbidden(authz.AssetAuthorize)
			}
			setAuth = in.TestingAuthorized
			changed = append(changed, "testing_authorized")
		}
		var attrs []byte
		if len(in.Attributes) > 0 {
			attrs = []byte(in.Attributes)
		}
		desc := in.Description
		if _, err := tx.Exec(ctx, `
UPDATE assets SET
  name = COALESCE($3, name),
  description = CASE WHEN $4::text IS NULL THEN description WHEN $4 = '' THEN NULL ELSE $4 END,
  platform = CASE WHEN $5 THEN NULL ELSE COALESCE($6, platform) END,
  arch = CASE WHEN $7 THEN NULL ELSE COALESCE($8, arch) END,
  criticality = COALESCE($9, criticality),
  is_internet_facing = COALESCE($10, is_internet_facing),
  attributes = COALESCE($11::jsonb, attributes),
  testing_authorized = COALESCE($12, testing_authorized),
  authorized_by = CASE WHEN $12::boolean IS NULL THEN authorized_by WHEN $12 THEN $13::uuid ELSE NULL END,
  authorized_at = CASE WHEN $12::boolean IS NULL THEN authorized_at WHEN $12 THEN now() ELSE NULL END,
  version = version + 1, updated_at = now()
WHERE tenant_id=$1 AND id=$2`,
			p.TenantID, id, in.Name, desc, in.ClearPlatform, in.Platform, in.ClearArch, in.Arch, in.Criticality,
			in.IsInternetFacing, attrs, setAuth, p.UserID); err != nil {
			return err
		}
		for name, set := range map[string]bool{"name": in.Name != nil, "description": in.Description != nil, "platform": in.Platform != nil || in.ClearPlatform,
			"arch": in.Arch != nil || in.ClearArch, "criticality": in.Criticality != nil, "is_internet_facing": in.IsInternetFacing != nil,
			"attributes": attrs != nil, "addresses": in.Addresses != nil, "tags": in.TagIDs != nil} {
			if set {
				changed = append(changed, name)
			}
		}
		if in.Addresses != nil {
			if _, err := tx.Exec(ctx, `DELETE FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2`, p.TenantID, id); err != nil {
				return err
			}
			for i, ad := range *in.Addresses {
				if _, err := tx.Exec(ctx, `INSERT INTO asset_addresses (id, tenant_id, asset_id, kind, value, is_primary) VALUES ($1,$2,$3,$4,$5,$6)`,
					ids.New(), p.TenantID, id, ad.Kind, ad.Value, ad.IsPrimary || i == 0); err != nil {
					return err
				}
			}
		}
		if in.TagIDs != nil {
			if err := setTags(ctx, tx, p.TenantID, id, *in.TagIDs); err != nil {
				return err
			}
		}
		if err := groups.RecomputeAll(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset.update", ResourceType: "asset",
			ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"fields": changed}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

// Delete menghapus lunak aset: riwayat tetap merujuknya, tetapi tidak lagi tampil dan keluar dari semua grup.
func (s *Service) Delete(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `UPDATE assets SET deleted_at = now(), updated_at = now(), version = version + 1
WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL RETURNING name`, p.TenantID, id).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Asset")
		}
		if err != nil {
			return err
		}
		if err := groups.RecomputeAll(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset.delete", ResourceType: "asset",
			ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": name}})
	})
}
