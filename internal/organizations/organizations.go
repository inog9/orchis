// Package organizations mengelompokkan aset dan assessment tanpa mengubah batas tenant.
package organizations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"orchis-go/internal/scancontrol"
	"regexp"
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
	"orchis-go/internal/simulation"
)

type Service struct {
	Now    func() time.Time
	DB     *db.DB
	Engine *simulation.Engine
}
type Organization struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Industry    string    `json:"industry"`
	Contact     string    `json:"contact"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Input struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Industry    string `json:"industry"`
	Contact     string `json:"contact"`
	Version     int    `json:"version"`
}

const columns = `id,name,slug,description,industry,contact,version,created_at,updated_at`

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func invalid(field, msg string) error {
	return httpx.Validation(httpx.FieldError{Name: field, Message: msg})
}
func scan(row pgx.Row) (Organization, error) {
	var o Organization
	err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.Description, &o.Industry, &o.Contact, &o.Version, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}
func unique(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return httpx.Conflict("This slug or agent instance name is already in use.")
	}
	return err
}
func (s *Service) Save(ctx context.Context, p *auth.Principal, id uuid.UUID, in Input, ip, req string) (*Organization, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	if len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 200 {
		return nil, invalid("name", "Use 1 to 200 characters.")
	}
	if len(in.Slug) > 100 || !slugRE.MatchString(in.Slug) {
		return nil, invalid("slug", "Use lowercase letters, digits and single hyphens.")
	}
	if len(in.Description) > 4000 || len(in.Industry) > 200 || len(in.Contact) > 500 {
		return nil, invalid("description", "Organization details are too long.")
	}
	create := id == uuid.Nil
	if create {
		id = ids.New()
	} else if in.Version < 1 {
		return nil, invalid("version", "A current version is required.")
	}
	var out Organization
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var err error
		if create {
			out, err = scan(tx.QueryRow(ctx, `INSERT INTO organizations(id,tenant_id,name,slug,description,industry,contact) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+columns, id, p.TenantID, in.Name, in.Slug, in.Description, in.Industry, in.Contact))
		} else {
			var current int
			if err = tx.QueryRow(ctx, `SELECT version FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id).Scan(&current); errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound("Organization")
			}
			if err != nil {
				return err
			}
			if current != in.Version {
				return httpx.Conflict("This organization was changed. Reload and try again.")
			}
			out, err = scan(tx.QueryRow(ctx, `UPDATE organizations SET name=$3,slug=$4,description=$5,industry=$6,contact=$7,version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 RETURNING `+columns, p.TenantID, id, in.Name, in.Slug, in.Description, in.Industry, in.Contact))
		}
		if err != nil {
			return unique(err)
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "organization.save", ResourceType: "organization", ResourceID: &id, IP: ip, RequestID: req})
	})
	return &out, err
}

type Page struct {
	Items      []Organization `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

// Cursor UUIDv7 mengikuti urutan pembuatan dan tidak bergantung pada nama yang dapat berubah.
func cursor(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil {
		return uuid.Nil, httpx.BadRequest("Invalid cursor.")
	}
	id, e := uuid.Parse(string(b))
	if e != nil {
		return uuid.Nil, httpx.BadRequest("Invalid cursor.")
	}
	return id, nil
}
func (s *Service) List(ctx context.Context, p *auth.Principal, limit int, raw string) (Page, error) {
	out := Page{Items: []Organization{}}
	id, err := cursor(raw)
	if err != nil {
		return out, err
	}
	if limit < 1 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	err = s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+columns+` FROM organizations WHERE tenant_id=$1 AND ($2::uuid='00000000-0000-0000-0000-000000000000' OR id<$2) ORDER BY id DESC LIMIT $3`, p.TenantID, id, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			o, e := scan(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, o)
		}
		return rows.Err()
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		c := base64.RawURLEncoding.EncodeToString([]byte(out.Items[limit-1].ID.String()))
		out.NextCursor = &c
	}
	return out, err
}
func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Organization, error) {
	var o Organization
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var e error
		o, e = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM organizations WHERE tenant_id=$1 AND id=$2`, p.TenantID, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Organization")
		}
		return e
	})
	return &o, err
}

type MembershipInput struct {
	Assets []struct {
		ID      uuid.UUID `json:"id"`
		Version int       `json:"version"`
	} `json:"assets"`
	Authorized *bool `json:"testing_authorized"`
	Unassign   bool  `json:"unassign"`
}

// Pemindahan dan otorisasi massal bersifat atomik; agent aktif mengikuti organisasi asetnya.
func (s *Service) Members(ctx context.Context, p *auth.Principal, org uuid.UUID, in MembershipInput, ip, req string) error {
	if !authz.Has(p.Permissions, authz.AssetWrite) {
		return httpx.Forbidden(authz.AssetWrite)
	}
	if in.Authorized != nil && !authz.Has(p.Permissions, authz.AssetAuthorize) {
		return httpx.Forbidden(authz.AssetAuthorize)
	}
	if len(in.Assets) < 1 || len(in.Assets) > 100 {
		return invalid("assets", "Select 1 to 100 assets.")
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE tenant_id=$1 AND id=$2)`, p.TenantID, org).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound("Organization")
		}
		var dest *uuid.UUID
		if !in.Unassign {
			dest = &org
		}
		seen := map[uuid.UUID]bool{}
		for _, a := range in.Assets {
			if seen[a.ID] {
				return invalid("assets", "Duplicate asset.")
			}
			seen[a.ID] = true
			tag, err := tx.Exec(ctx, `UPDATE assets SET organization_id=$3, testing_authorized=COALESCE($4,testing_authorized),authorized_by=CASE WHEN $4::bool IS NULL THEN authorized_by WHEN $4 THEN $5::uuid ELSE NULL END,authorized_at=CASE WHEN $4::bool IS NULL THEN authorized_at WHEN $4 THEN now() ELSE NULL END,version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND version=$6 AND deleted_at IS NULL`, p.TenantID, a.ID, dest, in.Authorized, p.UserID, a.Version)
			if err != nil {
				return unique(err)
			}
			if tag.RowsAffected() != 1 {
				return httpx.Conflict("An asset was changed or no longer exists. Reload and try again.")
			}
			if _, err = tx.Exec(ctx, `UPDATE agents SET organization_id=$3,updated_at=now() WHERE tenant_id=$1 AND asset_id=$2`, p.TenantID, a.ID, dest); err != nil {
				return unique(err)
			}
		}
		detail, _ := json.Marshal(in)
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "organization.assets", ResourceType: "organization", ResourceID: &org, IP: ip, RequestID: req, Detail: map[string]any{"changes": json.RawMessage(detail)}})
	})
}
