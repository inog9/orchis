package agents

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/agentproto"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/scancontrol"
	"time"
)

type RolloutPolicy struct {
	Mode    string `json:"mode"`
	Version int    `json:"version"`
}

func adminControl(p *auth.Principal) error {
	for _, permission := range []string{authz.SettingsManage, authz.AgentManage} {
		if !authz.Has(p.Permissions, permission) {
			return httpx.Forbidden(permission)
		}
	}
	return nil
}
func rollout(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (RolloutPolicy, error) {
	out := RolloutPolicy{Mode: "automatic"}
	err := tx.QueryRow(ctx, `SELECT mode,version FROM agent_rollout_policies WHERE tenant_id=$1`, tenant).Scan(&out.Mode, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return out, err
}
func (s *Service) Rollout(ctx context.Context, p *auth.Principal) (*RolloutPolicy, error) {
	if err := adminControl(p); err != nil {
		return nil, err
	}
	var out RolloutPolicy
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error { var e error; out, e = rollout(ctx, tx, p.TenantID); return e })
	return &out, err
}
func (s *Service) SetRollout(ctx context.Context, p *auth.Principal, in RolloutPolicy, ip, req string) (*RolloutPolicy, error) {
	if err := adminControl(p); err != nil {
		return nil, err
	}
	if (in.Mode != "automatic" && in.Mode != "manual") || in.Version < 0 {
		return nil, fe("mode", "Choose automatic or manual rollout with the current version.")
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		current, err := rollout(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return httpx.Conflict("Rollout policy changed; reload before saving.")
		}
		in.Version++
		if _, err = tx.Exec(ctx, `INSERT INTO agent_rollout_policies(tenant_id,mode,version) VALUES($1,$2,$3) ON CONFLICT(tenant_id) DO UPDATE SET mode=EXCLUDED.mode,version=EXCLUDED.version,updated_at=now()`, p.TenantID, in.Mode, in.Version); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "agent.rollout.update", ResourceType: "tenant", ResourceID: &p.TenantID, IP: ip, RequestID: req, Detail: map[string]any{"mode": in.Mode, "version": in.Version}})
	})
	return &in, err
}

type ControlInput struct {
	Action       string `json:"action"`
	Version      int    `json:"version"`
	Confirmation string `json:"confirmation,omitempty"`
}

func (s *Service) Control(ctx context.Context, p *auth.Principal, id uuid.UUID, in ControlInput, ip, req string) (*Agent, error) {
	if err := adminControl(p); err != nil {
		return nil, err
	}
	if in.Version < 1 {
		return nil, fe("version", "Reload the agent before sending a command.")
	}
	var out *Agent
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		var current int
		var status, version, osName, arch, uninstall string
		var capability int
		err := tx.QueryRow(ctx, `SELECT control_version,status,version,os,arch,control_protocol,uninstall_status FROM agents WHERE tenant_id=$1 AND id=$2 AND NOT is_demo FOR UPDATE`, p.TenantID, id).Scan(&current, &status, &version, &osName, &arch, &capability, &uninstall)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Agent")
		}
		if err != nil {
			return err
		}
		if current != in.Version {
			return httpx.Conflict("Agent controls changed; reload before sending a command.")
		}
		if status == "revoked" {
			return httpx.Conflict("This agent is revoked.")
		}
		if uninstall == "queued" || uninstall == "dispatched" {
			return httpx.Conflict("Uninstall is pending; wait for the agent to confirm removal.")
		}
		detail := map[string]any{"version": current + 1}
		switch in.Action {
		case "pause", "resume":
			_, err = tx.Exec(ctx, `UPDATE agents SET paused=$3,control_version=control_version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, in.Action == "pause")
		case "update":
			m, e := s.Release.Manifest()
			if e != nil {
				return httpx.Conflict("No agent release is available.")
			}
			if _, ok := m.Find(osName, arch); !ok {
				return httpx.Conflict("No release for this platform.")
			}
			if !agentproto.Newer(m.Version, version) {
				return httpx.Conflict("The installed agent is already current or newer; downgrades are refused.")
			}
			detail["release_version"] = m.Version
			_, err = tx.Exec(ctx, `UPDATE agents SET requested_update_version=$3,control_version=control_version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, m.Version)
		case "uninstall":
			if in.Confirmation != id.String() {
				return fe("confirmation", "Enter the exact agent ID to confirm uninstall.")
			}
			if capability != agentproto.ControlProtocol {
				return httpx.Conflict("Update this agent before remote uninstall; it has not advertised support.")
			}
			command := uuid.New()
			detail["command_id"] = command
			_, err = tx.Exec(ctx, `UPDATE agents SET paused=true,uninstall_id=$3,uninstall_status='queued',uninstall_requested_at=$4,uninstall_completed_at=NULL,control_version=control_version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, command, s.now())
		default:
			return fe("action", "Choose update, pause, resume or uninstall.")
		}
		if err != nil {
			return err
		}
		if err = audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "agent." + in.Action, ResourceType: "agent", ResourceID: &id, IP: ip, RequestID: req, Detail: detail}); err != nil {
			return err
		}
		rows, err := s.list(ctx, tx, p.TenantID, &id)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return httpx.NotFound("Agent")
		}
		out = &rows[0]
		return nil
	})
	return out, err
}

// Ack is accepted only for this authenticated agent and its current typed command.
// Completion revokes the credential in the same transaction. Lost completion ACKs
// remain visibly unconfirmed; absence of contact never proves physical removal.
func (s *Service) Ack(ctx context.Context, p *Principal, in agentproto.ControlAck, ip string) error {
	command, e := uuid.Parse(in.ID)
	if e != nil {
		return fe("id", "Invalid command ID.")
	}
	return s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, s.Tenant); e != nil {
			return e
		}
		var current *uuid.UUID
		var state string
		if e := tx.QueryRow(ctx, `SELECT uninstall_id,uninstall_status FROM agents WHERE tenant_id=$1 AND id=$2 AND status<>'revoked' FOR UPDATE`, s.Tenant, p.AgentID).Scan(&current, &state); errors.Is(e, pgx.ErrNoRows) {
			return httpx.Unauthorized()
		} else if e != nil {
			return e
		}
		if current == nil || *current != command || (state != "queued" && state != "dispatched" && state != "failed") {
			return httpx.Conflict("This uninstall command is no longer current.")
		}
		status := "failed"
		code := "uninstall_failed"
		var completed *time.Time
		if in.Success {
			status = "completed"
			code = ""
			now := s.now()
			completed = &now
		}
		if _, e := tx.Exec(ctx, `UPDATE agents SET uninstall_status=$3,uninstall_completed_at=$4,diagnostic_code=$5,diagnostic_at=$6,control_version=control_version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, s.Tenant, p.AgentID, status, completed, code, s.now()); e != nil {
			return e
		}
		if in.Success {
			if e := s.revokeTx(ctx, tx, s.Tenant, p.AgentID, nil); e != nil {
				return e
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: s.Tenant, Action: "agent.uninstall.ack", ResourceType: "agent", ResourceID: &p.AgentID, IP: ip, Detail: map[string]any{"command_id": command, "success": in.Success, "code": code}})
	})
}
