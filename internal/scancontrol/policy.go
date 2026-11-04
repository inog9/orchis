package scancontrol

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/httpx"
	"time"
)

// Lock serializes scan state transitions and emergency stops within one tenant.
// It must be acquired before execution/simulation/job row locks. Nested calls in
// the same transaction reuse PostgreSQL's transaction advisory lock.
func Lock(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "scan-control:"+tenant.String())
	return e
}

type Blocked struct{ Code, Reason string }

func (b *Blocked) Error() string { return b.Reason }
func (b *Blocked) Unwrap() error { return httpx.Conflict(b.Reason) }

type State struct {
	GlobalPausedBy string     `json:"global_paused_by,omitempty"`
	AllowedNow     bool       `json:"allowed_now"`
	Restriction    string     `json:"restriction,omitempty"`
	OrganizationID *uuid.UUID `json:"organization_id"`
	Paused         bool       `json:"paused"`
	GlobalPaused   bool       `json:"global_paused"`
	PausedBy       string     `json:"paused_by,omitempty"`
	Window         Window     `json:"window"`
	Version        int        `json:"version"`
}

// Read never creates a control record. Version zero is the CAS value for defaults.
func Read(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, org *uuid.UUID) (State, error) {
	out := State{OrganizationID: org}
	rows, e := tx.Query(ctx, `SELECT c.organization_id,c.paused,COALESCE(u.display_name,'Administrator'),c.scan_window,c.version FROM scan_controls c LEFT JOIN users u ON u.tenant_id=c.tenant_id AND u.id=c.paused_by WHERE c.tenant_id=$1 AND (c.organization_id IS NULL OR c.organization_id=$2) ORDER BY c.organization_id NULLS FIRST`, tenant, org)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id *uuid.UUID
		var paused bool
		var actor string
		var raw []byte
		var version int
		if e = rows.Scan(&id, &paused, &actor, &raw, &version); e != nil {
			return out, e
		}
		if id == nil {
			out.GlobalPaused = paused
			if paused {
				out.GlobalPausedBy = actor
			}
			if org == nil {
				out.Paused = paused
				out.PausedBy = actor
				out.Version = version
			}
		} else {
			out.Paused = paused
			out.Version = version
			if paused {
				out.PausedBy = actor
			}
			if e = json.Unmarshal(raw, &out.Window); e != nil {
				return out, e
			}
		}
		if id == nil && paused {
			out.PausedBy = actor
		}
	}
	return out, rows.Err()
}

func Check(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, org *uuid.UUID, now time.Time) error {
	state, e := Read(ctx, tx, tenant, org)
	if e != nil {
		return e
	}
	if state.GlobalPaused || state.Paused {
		actor := state.PausedBy
		if state.GlobalPaused {
			actor = state.GlobalPausedBy
		}
		return &Blocked{Code: "emergency_stop", Reason: "Scans are held after an emergency stop by " + actor + ". An administrator must resume scanning."}
	}
	allowed, e := state.Window.Allows(now)
	if e != nil {
		return &Blocked{Code: "invalid_window", Reason: "Scanning is held because the configured scan window is invalid."}
	}
	if !allowed {
		return &Blocked{Code: "outside_window", Reason: "Stopped because the organisation scan window is closed."}
	}
	return nil
}

func CheckAsset(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, frozen *uuid.UUID, now time.Time) error {
	var current *uuid.UUID
	e := tx.QueryRow(ctx, `SELECT organization_id FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, tenant, asset).Scan(&current)
	if errors.Is(e, pgx.ErrNoRows) {
		return &Blocked{Code: "asset_removed", Reason: "Stopped because the target asset no longer exists."}
	}
	if e != nil {
		return e
	}
	if e = Check(ctx, tx, tenant, frozen, now); e != nil {
		return e
	}
	if current != nil && (frozen == nil || *current != *frozen) {
		return Check(ctx, tx, tenant, current, now)
	}
	return nil
}
