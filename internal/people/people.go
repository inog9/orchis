// Package people: peserta latihan (orang) dan tim. Bukan akun login (lihat ADR-008). Tanpa alur tabletop atau email/SMS pada MVP.
package people

import (
	"context"
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

type Service struct{ DB *db.DB }

type TeamRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Person struct {
	ID           uuid.UUID `json:"id"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	Email        *string   `json:"email"`
	Organization *string   `json:"organization"`
	Teams        []TeamRef `json:"teams"`
	CreatedAt    time.Time `json:"created_at"`
	Version      int       `json:"version"`
}

type Team struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Kind        string    `json:"kind"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
	Version     int       `json:"version"`
}

type MemberRef struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email *string   `json:"email"`
}

type TeamDetail struct {
	Team
	Members []MemberRef `json:"members"`
}

func fe(name, msg string) error { return httpx.Validation(httpx.FieldError{Name: name, Message: msg}) }

func dup(err error, field, msg string) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return fe(field, msg)
	}
	return err
}

func trimPtr(p *string, max int, field string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > max {
		return nil, fe(field, "Too long.")
	}
	return &v, nil
}

// ---------- orang ----------

type PersonInput struct {
	FirstName    string  `json:"first_name"`
	LastName     string  `json:"last_name"`
	Email        *string `json:"email"`
	Organization *string `json:"organization"`
}

func (in *PersonInput) clean() (email, org *string, err error) {
	in.FirstName, in.LastName = strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName)
	if in.FirstName == "" || len([]rune(in.FirstName)) > 80 {
		return nil, nil, fe("first_name", "Enter a first name of 1 to 80 characters.")
	}
	if len([]rune(in.LastName)) > 80 {
		return nil, nil, fe("last_name", "At most 80 characters.")
	}
	if email, err = trimPtr(in.Email, 254, "email"); err != nil {
		return
	}
	if email != nil && (!strings.Contains(*email, "@") || strings.ContainsAny(*email, " <>")) {
		return nil, nil, fe("email", "Enter a valid email address.")
	}
	org, err = trimPtr(in.Organization, 120, "organization")
	return
}

func loadTeams(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ps []Person) error {
	if len(ps) == 0 {
		return nil
	}
	idx := map[uuid.UUID]int{}
	idList := make([]uuid.UUID, len(ps))
	for i := range ps {
		idx[ps[i].ID] = i
		idList[i] = ps[i].ID
		ps[i].Teams = []TeamRef{}
	}
	rows, err := tx.Query(ctx, `SELECT m.person_id, t.id, t.name FROM team_members m JOIN teams t ON t.tenant_id=m.tenant_id AND t.id=m.team_id AND t.deleted_at IS NULL
WHERE m.tenant_id=$1 AND m.person_id = ANY($2) ORDER BY t.name`, tenant, idList)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid uuid.UUID
		var tr TeamRef
		if err := rows.Scan(&pid, &tr.ID, &tr.Name); err != nil {
			return err
		}
		ps[idx[pid]].Teams = append(ps[idx[pid]].Teams, tr)
	}
	return rows.Err()
}

const personCols = `p.id, p.first_name, p.last_name, p.email::text, p.organization, p.created_at, p.version`

func scanPerson(row pgx.Row) (Person, error) {
	var p Person
	err := row.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Email, &p.Organization, &p.CreatedAt, &p.Version)
	return p, err
}

func (s *Service) ListPeople(ctx context.Context, p *auth.Principal, q string) ([]Person, error) {
	out := []Person{}
	q = strings.TrimSpace(q)
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		sql := `SELECT ` + personCols + ` FROM people p WHERE p.tenant_id=$1 AND p.deleted_at IS NULL`
		args := []any{p.TenantID}
		if q != "" {
			sql += ` AND (p.first_name || ' ' || p.last_name ILIKE $2 OR p.email::text ILIKE $2 OR p.organization ILIKE $2)`
			args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%")
		}
		rows, err := tx.Query(ctx, sql+` ORDER BY p.first_name, p.last_name, p.id LIMIT 500`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			x, err := scanPerson(rows)
			if err != nil {
				rows.Close()
				return err
			}
			out = append(out, x)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		return loadTeams(ctx, tx, p.TenantID, out)
	})
	return out, err
}

func (s *Service) GetPerson(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Person, error) {
	var out *Person
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		x, err := scanPerson(tx.QueryRow(ctx, `SELECT `+personCols+` FROM people p WHERE p.tenant_id=$1 AND p.id=$2 AND p.deleted_at IS NULL`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Person")
		}
		if err != nil {
			return err
		}
		list := []Person{x}
		if err := loadTeams(ctx, tx, p.TenantID, list); err != nil {
			return err
		}
		out = &list[0]
		return nil
	})
	return out, err
}

func (s *Service) CreatePerson(ctx context.Context, p *auth.Principal, in PersonInput, ip, reqID string) (*Person, error) {
	email, org, err := in.clean()
	if err != nil {
		return nil, err
	}
	id := ids.New()
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO people (id, tenant_id, first_name, last_name, email, organization) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, p.TenantID, in.FirstName, in.LastName, email, org); err != nil {
			return dup(err, "email", "Someone with this email already exists.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "person.create", ResourceType: "person", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPerson(ctx, p, id)
}

type PersonPatch struct {
	Version int `json:"version"`
	PersonInput
}

func (s *Service) PatchPerson(ctx context.Context, p *auth.Principal, id uuid.UUID, in PersonPatch, ip, reqID string) (*Person, error) {
	if in.Version < 1 {
		return nil, fe("version", "Send the version you loaded.")
	}
	email, org, err := in.clean()
	if err != nil {
		return nil, err
	}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE people SET first_name=$4, last_name=$5, email=$6, organization=$7, version=version+1, updated_at=now()
WHERE tenant_id=$1 AND id=$2 AND version=$3 AND deleted_at IS NULL`, p.TenantID, id, in.Version, in.FirstName, in.LastName, email, org)
		if err != nil {
			return dup(err, "email", "Someone with this email already exists.")
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM people WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, p.TenantID, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.NotFound("Person")
			}
			return httpx.Conflict("This person was changed by someone else. Reload and try again.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "person.update", ResourceType: "person", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPerson(ctx, p, id)
}

func (s *Service) DeletePerson(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE people SET deleted_at=now(), version=version+1 WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, p.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Person")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM team_members WHERE tenant_id=$1 AND person_id=$2`, p.TenantID, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "person.delete", ResourceType: "person", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}

// ---------- tim ----------

type TeamInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Kind        string  `json:"kind"`
}

func (in *TeamInput) clean() (*string, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return nil, fe("name", "Enter a name of 1 to 120 characters.")
	}
	if in.Kind == "" {
		in.Kind = "internal"
	}
	if in.Kind != "internal" && in.Kind != "external" {
		return nil, fe("kind", "Choose internal or external.")
	}
	return trimPtr(in.Description, 1000, "description")
}

const teamCols = `t.id, t.name, t.description, t.kind, t.created_at, t.version,
 (SELECT count(*) FROM team_members m JOIN people p ON p.tenant_id=m.tenant_id AND p.id=m.person_id AND p.deleted_at IS NULL WHERE m.tenant_id=t.tenant_id AND m.team_id=t.id)`

func scanTeam(row pgx.Row) (Team, error) {
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.Kind, &t.CreatedAt, &t.Version, &t.MemberCount)
	return t, err
}

func (s *Service) ListTeams(ctx context.Context, p *auth.Principal) ([]Team, error) {
	out := []Team{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+teamCols+` FROM teams t WHERE t.tenant_id=$1 AND t.deleted_at IS NULL ORDER BY t.name LIMIT 500`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTeam(rows)
			if err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) GetTeam(ctx context.Context, p *auth.Principal, id uuid.UUID) (*TeamDetail, error) {
	var out *TeamDetail
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		t, err := scanTeam(tx.QueryRow(ctx, `SELECT `+teamCols+` FROM teams t WHERE t.tenant_id=$1 AND t.id=$2 AND t.deleted_at IS NULL`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Team")
		}
		if err != nil {
			return err
		}
		d := &TeamDetail{Team: t, Members: []MemberRef{}}
		rows, err := tx.Query(ctx, `SELECT p.id, trim(p.first_name || ' ' || p.last_name), p.email::text FROM team_members m
JOIN people p ON p.tenant_id=m.tenant_id AND p.id=m.person_id AND p.deleted_at IS NULL WHERE m.tenant_id=$1 AND m.team_id=$2 ORDER BY p.first_name, p.last_name`, p.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m MemberRef
			if err := rows.Scan(&m.ID, &m.Name, &m.Email); err != nil {
				return err
			}
			d.Members = append(d.Members, m)
		}
		out = d
		return rows.Err()
	})
	return out, err
}

func (s *Service) CreateTeam(ctx context.Context, p *auth.Principal, in TeamInput, ip, reqID string) (*TeamDetail, error) {
	desc, err := in.clean()
	if err != nil {
		return nil, err
	}
	id := ids.New()
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO teams (id, tenant_id, name, description, kind) VALUES ($1,$2,$3,$4,$5)`, id, p.TenantID, in.Name, desc, in.Kind); err != nil {
			return dup(err, "name", "A team with this name already exists.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "team.create", ResourceType: "team", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetTeam(ctx, p, id)
}

type TeamPatch struct {
	Version int `json:"version"`
	TeamInput
}

func (s *Service) PatchTeam(ctx context.Context, p *auth.Principal, id uuid.UUID, in TeamPatch, ip, reqID string) (*TeamDetail, error) {
	if in.Version < 1 {
		return nil, fe("version", "Send the version you loaded.")
	}
	desc, err := in.TeamInput.clean()
	if err != nil {
		return nil, err
	}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE teams SET name=$4, description=$5, kind=$6, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND version=$3 AND deleted_at IS NULL`,
			p.TenantID, id, in.Version, in.Name, desc, in.Kind)
		if err != nil {
			return dup(err, "name", "A team with this name already exists.")
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, p.TenantID, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.NotFound("Team")
			}
			return httpx.Conflict("This team was changed by someone else. Reload and try again.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "team.update", ResourceType: "team", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetTeam(ctx, p, id)
}

func (s *Service) DeleteTeam(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE teams SET deleted_at=now(), version=version+1 WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, p.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Team")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "team.delete", ResourceType: "team", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}

type MembersInput struct {
	Version   int         `json:"version"`
	PersonIDs []uuid.UUID `json:"person_ids"`
}

// SetMembers mengganti seluruh anggota tim secara atomik.
func (s *Service) SetMembers(ctx context.Context, p *auth.Principal, id uuid.UUID, in MembersInput, ip, reqID string) (*TeamDetail, error) {
	if in.Version < 1 {
		return nil, fe("version", "Send the version you loaded.")
	}
	if len(in.PersonIDs) > 1000 {
		return nil, fe("person_ids", "At most 1000 members.")
	}
	seen := map[uuid.UUID]bool{}
	uniq := []uuid.UUID{}
	for _, x := range in.PersonIDs {
		if !seen[x] {
			seen[x] = true
			uniq = append(uniq, x)
		}
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var cur int
		if err := tx.QueryRow(ctx, `SELECT version FROM teams WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, p.TenantID, id).Scan(&cur); errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Team")
		} else if err != nil {
			return err
		}
		if cur != in.Version {
			return httpx.Conflict("This team was changed by someone else. Reload and try again.")
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM people WHERE tenant_id=$1 AND deleted_at IS NULL AND id = ANY($2)`, p.TenantID, uniq).Scan(&n); err != nil {
			return err
		}
		if n != len(uniq) {
			return fe("person_ids", "One of the people does not exist.")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM team_members WHERE tenant_id=$1 AND team_id=$2`, p.TenantID, id); err != nil {
			return err
		}
		for _, x := range uniq {
			if _, err := tx.Exec(ctx, `INSERT INTO team_members (tenant_id, team_id, person_id) VALUES ($1,$2,$3)`, p.TenantID, id, x); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE teams SET version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "team.members", ResourceType: "team", ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"members": len(uniq)}})
	})
	if err != nil {
		return nil, err
	}
	return s.GetTeam(ctx, p, id)
}

// ---------- HTTP ----------

type Handlers struct{ Svc *Service }

func pid(r *http.Request, what string) (uuid.UUID, error) {
	v, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return uuid.Nil, httpx.NotFound(what)
	}
	return v, nil
}

func reply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}

func body[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var in T
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return in, false
	}
	return in, true
}

func (h *Handlers) Register(mux *http.ServeMux) {
	ip := func(r *http.Request) (string, string) { return auth.ClientIP(r), httpx.RequestIDFrom(r.Context()) }
	mux.HandleFunc("GET /api/v1/people", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.ListPeople(r.Context(), p, r.URL.Query().Get("q"))
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/people", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		in, ok := body[PersonInput](w, r)
		if !ok {
			return
		}
		a, b := ip(r)
		v, err := h.Svc.CreatePerson(r.Context(), p, in, a, b)
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("GET /api/v1/people/{id}", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Person")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.GetPerson(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("PATCH /api/v1/people/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Person")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		in, ok := body[PersonPatch](w, r)
		if !ok {
			return
		}
		a, b := ip(r)
		v, err := h.Svc.PatchPerson(r.Context(), p, id, in, a, b)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("DELETE /api/v1/people/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Person")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		a, b := ip(r)
		if err := h.Svc.DeletePerson(r.Context(), p, id, a, b); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/v1/teams", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.ListTeams(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/teams", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		in, ok := body[TeamInput](w, r)
		if !ok {
			return
		}
		a, b := ip(r)
		v, err := h.Svc.CreateTeam(r.Context(), p, in, a, b)
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("GET /api/v1/teams/{id}", auth.Require(authz.PeopleRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Team")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.GetTeam(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("PATCH /api/v1/teams/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Team")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		in, ok := body[TeamPatch](w, r)
		if !ok {
			return
		}
		a, b := ip(r)
		v, err := h.Svc.PatchTeam(r.Context(), p, id, in, a, b)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("DELETE /api/v1/teams/{id}", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Team")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		a, b := ip(r)
		if err := h.Svc.DeleteTeam(r.Context(), p, id, a, b); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("PUT /api/v1/teams/{id}/members", auth.Require(authz.PeopleWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Team")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		in, ok := body[MembersInput](w, r)
		if !ok {
			return
		}
		a, b := ip(r)
		v, err := h.Svc.SetMembers(r.Context(), p, id, in, a, b)
		reply(w, r, 200, v, err)
	}))
}
