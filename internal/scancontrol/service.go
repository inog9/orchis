package scancontrol

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"time"
)

type Service struct {
	DB      *db.DB
	Now     func() time.Time
	Advance Advance
}
type Input struct {
	Version int    `json:"version"`
	Window  Window `json:"window"`
}

func exists(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, org *uuid.UUID) error {
	if org == nil {
		return nil
	}
	var found bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE tenant_id=$1 AND id=$2 AND NOT is_demo)`, tenant, *org).Scan(&found); e != nil {
		return e
	}
	if !found {
		return httpx.NotFound("Organisation")
	}
	return nil
}
func permission(p *auth.Principal, org *uuid.UUID, write bool) error {
	perm := authz.OrganizationRead
	if write || org == nil {
		perm = authz.SettingsManage
	}
	if !authz.Has(p.Permissions, perm) {
		return httpx.Forbidden(perm)
	}
	return nil
}
func (s *Service) Get(ctx context.Context, p *auth.Principal, org *uuid.UUID) (State, error) {
	var state State
	if e := permission(p, org, false); e != nil {
		return state, e
	}
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := exists(ctx, tx, p.TenantID, org); e != nil {
			return e
		}
		var e error
		state, e = Read(ctx, tx, p.TenantID, org)
		return e
	})
	return s.present(state), e
}

// Change supports window, stop and resume. Emergency stop deliberately ignores a
// stale version: stopping takes priority. Window and resume use CAS; resume does
// not restart interrupted jobs. Repeated stop preserves the first actor/audit.
func (s *Service) Change(ctx context.Context, p *auth.Principal, org *uuid.UUID, action string, in Input, ip, req string) (State, error) {
	var state State
	if e := permission(p, org, true); e != nil {
		return state, e
	}
	if action != "window" && action != "stop" && action != "resume" {
		return state, httpx.BadRequest("Unknown scan control action.")
	}
	if action == "window" {
		if org == nil {
			return state, httpx.BadRequest("Choose an organisation.")
		}
		if e := in.Window.Validate(); e != nil {
			return state, httpx.Validation(httpx.FieldError{Name: "window", Message: e.Error()})
		}
	}
	e := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := Lock(ctx, tx, p.TenantID); e != nil {
			return e
		}
		if e := exists(ctx, tx, p.TenantID, org); e != nil {
			return e
		}
		current, e := Read(ctx, tx, p.TenantID, org)
		if e != nil {
			return e
		}
		if action != "stop" && current.Version != in.Version {
			return httpx.Conflict("Scan controls changed. Reload and try again.")
		}
		if action == "stop" && current.Paused {
			state = current
			return Enforce(ctx, tx, p.TenantID, Now(s.Now), s.Advance)
		}
		if action == "resume" && !current.Paused {
			state = current
			return nil
		}
		if current.Version == 0 {
			_, e = tx.Exec(ctx, `INSERT INTO scan_controls(id,tenant_id,organization_id) VALUES($1,$2,$3)`, ids.New(), p.TenantID, org)
			if e != nil {
				return e
			}
		}
		switch action {
		case "window":
			_, e = tx.Exec(ctx, `UPDATE scan_controls SET scan_window=$3,version=version+1,updated_at=now() WHERE tenant_id=$1 AND organization_id=$2`, p.TenantID, org, in.Window)
		case "stop":
			_, e = tx.Exec(ctx, `UPDATE scan_controls SET paused=true,paused_by=$3,paused_at=now(),version=version+1,updated_at=now() WHERE tenant_id=$1 AND organization_id IS NOT DISTINCT FROM $2::uuid`, p.TenantID, org, p.UserID)
		case "resume":
			_, e = tx.Exec(ctx, `UPDATE scan_controls SET paused=false,paused_by=NULL,paused_at=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND organization_id IS NOT DISTINCT FROM $2::uuid`, p.TenantID, org)
		}
		if e != nil {
			return e
		}
		if e = Enforce(ctx, tx, p.TenantID, Now(s.Now), s.Advance); e != nil {
			return e
		}
		if e = audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scan_control." + action, ResourceType: "organization", ResourceID: org, IP: ip, RequestID: req, Detail: map[string]any{"global": org == nil, "window": in.Window}}); e != nil {
			return e
		}
		state, e = Read(ctx, tx, p.TenantID, org)
		return e
	})
	return s.present(state), e
}

func (s *Service) present(state State) State {
	state.AllowedNow = true
	if state.GlobalPaused || state.Paused {
		state.AllowedNow = false
		state.Restriction = "Scanning is held after an emergency stop. An administrator must resume scanning."
	} else if allowed, e := state.Window.Allows(Now(s.Now)); e != nil || !allowed {
		state.AllowedNow = false
		state.Restriction = "The organisation scan window is closed."
	}
	return state
}
