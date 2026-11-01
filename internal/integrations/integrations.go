// Package integrations: halaman status injector/collector/executor (heartbeat, log terbaru, token, aktif/nonaktif).
package integrations

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
)

const onlineWindow = 60 * time.Second

type LogLine struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type Integration struct {
	IsDemo          bool       `json:"is_demo"`
	ID              uuid.UUID  `json:"id"`
	Kind            string     `json:"kind"`
	Key             string     `json:"key"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	IsBuiltin       bool       `json:"is_builtin"`
	Enabled         bool       `json:"enabled"`
	Status          string     `json:"status"` // online | offline | disabled
	SoftwareVersion *string    `json:"software_version"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
	HasToken        bool       `json:"has_token"`
	Logs            []LogLine  `json:"logs"`
}

type Service struct{ DB *db.DB }

func (s *Service) List(ctx context.Context, p *auth.Principal) ([]Integration, error) {
	out := []Integration{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, kind, key, name, description, is_builtin, enabled, software_version, last_heartbeat_at, (config ? 'token_sha256'), is_demo, status
FROM integrations WHERE tenant_id=$1 ORDER BY array_position(ARRAY['injector','collector','executor'], kind), name`, p.TenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var i Integration
			var storedStatus string
			if err := rows.Scan(&i.ID, &i.Kind, &i.Key, &i.Name, &i.Description, &i.IsBuiltin, &i.Enabled, &i.SoftwareVersion, &i.LastHeartbeatAt, &i.HasToken, &i.IsDemo, &storedStatus); err != nil {
				rows.Close()
				return err
			}
			switch {
			case !i.Enabled:
				i.Status = "disabled"
			case i.IsDemo && storedStatus == "running":
				i.Status = "online"
			case !i.IsDemo && i.LastHeartbeatAt != nil && time.Since(*i.LastHeartbeatAt) < onlineWindow:
				i.Status = "online"
			default:
				i.Status = "offline"
			}
			i.Logs = []LogLine{}
			out = append(out, i)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for k := range out {
			lr, err := tx.Query(ctx, `SELECT at, level, message FROM integration_logs WHERE tenant_id=$1 AND integration_id=$2 ORDER BY at DESC, id DESC LIMIT 5`, p.TenantID, out[k].ID)
			if err != nil {
				return err
			}
			for lr.Next() {
				var l LogLine
				if err := lr.Scan(&l.At, &l.Level, &l.Message); err != nil {
					lr.Close()
					return err
				}
				out[k].Logs = append(out[k].Logs, l)
			}
			lr.Close()
			if err := lr.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func logLine(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, level, msg string) error {
	_, err := tx.Exec(ctx, `INSERT INTO integration_logs (tenant_id, integration_id, level, message) VALUES ($1,$2,$3,$4)`, tenant, id, level, msg)
	return err
}

func (s *Service) SetEnabled(ctx context.Context, p *auth.Principal, id uuid.UUID, enabled bool, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var key string
		err := tx.QueryRow(ctx, `UPDATE integrations SET enabled=$3, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND NOT is_demo RETURNING key`, p.TenantID, id, enabled).Scan(&key)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Integration")
		}
		if err != nil {
			return err
		}
		msg := "Disabled by " + p.Email + "."
		if enabled {
			msg = "Enabled by " + p.Email + "."
		}
		if err := logLine(ctx, tx, p.TenantID, id, "info", msg); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "integration.enable", ResourceType: "integration", ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"key": key, "enabled": enabled}})
	})
}

type NewToken struct {
	Token string `json:"token"` // hanya tampil sekali
}

// RotateToken membuat token baru; token lama langsung tidak berlaku.
func (s *Service) RotateToken(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) (*NewToken, error) {
	var key, kind string
	var had bool
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT key, kind, config ? 'token_sha256' FROM integrations WHERE tenant_id=$1 AND id=$2 AND NOT is_demo`, p.TenantID, id).Scan(&key, &kind, &had)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound("Integration")
	}
	if err != nil {
		return nil, err
	}
	if kind != "injector" {
		return nil, httpx.Conflict("Only injectors use tokens.")
	}
	tok, err := execution.IssueToken(ctx, s.DB, p.TenantID, key)
	if err != nil {
		return nil, err
	}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		msg, level := "Token created by "+p.Email+".", "info"
		if had {
			msg, level = "Token rotated by "+p.Email+". The previous token no longer works.", "warn"
		}
		if err := logLine(ctx, tx, p.TenantID, id, level, msg); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "integration.token_rotate", ResourceType: "integration", ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"key": key}})
	})
	if err != nil {
		return nil, err
	}
	return &NewToken{Token: tok}, nil
}

type Handlers struct{ Svc *Service }

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/integrations", auth.Require(authz.IntegrationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.List(r.Context(), p)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, map[string]any{"items": v})
	}))
	mux.HandleFunc("PATCH /api/v1/integrations/{id}", auth.Require(authz.IntegrationManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Integration"))
			return
		}
		var in struct {
			Enabled *bool `json:"enabled"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if in.Enabled == nil {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "enabled", Message: "Send true or false."}))
			return
		}
		if err := h.Svc.SetEnabled(r.Context(), p, id, *in.Enabled, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/v1/integrations/{id}/token", auth.Require(authz.IntegrationManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Integration"))
			return
		}
		v, err := h.Svc.RotateToken(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, v)
	}))
}
