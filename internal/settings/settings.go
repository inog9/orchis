// Package settings: parameter platform dan kesehatan komponen.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
)

const (
	KeyPlatformName = "platform.name"
	DefaultName     = "Platform default"
)

// Check memeriksa satu komponen infrastruktur.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

type Service struct {
	DB      *db.DB
	Version string
	Checks  []Check
}

type Component struct {
	Name   string `json:"name"`
	Status string `json:"status"` // up | down | not_configured
	Detail string `json:"detail,omitempty"`
}

type Settings struct {
	PlatformName string      `json:"platform_name"`
	Version      string      `json:"version"`
	Components   []Component `json:"components"` // hanya untuk pemegang settings.manage
}

func (s *Service) name(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (string, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT value FROM settings WHERE tenant_id=$1 AND key=$2`, tenant, KeyPlatformName).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultName, nil
	}
	if err != nil {
		return "", err
	}
	var v string
	if json.Unmarshal(raw, &v) != nil || v == "" {
		return DefaultName, nil
	}
	return v, nil
}

func (s *Service) Get(ctx context.Context, p *auth.Principal) (*Settings, error) {
	out := &Settings{Version: s.Version, Components: []Component{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		n, err := s.name(ctx, tx, p.TenantID)
		out.PlatformName = n
		return err
	})
	if err != nil || !authz.Has(p.Permissions, authz.SettingsManage) {
		return out, err
	}
	// Periksa komponen paralel dengan batas waktu supaya satu komponen yang macet tidak menahan halaman.
	res := make([]Component, len(s.Checks))
	var wg sync.WaitGroup
	for i, c := range s.Checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			res[i] = Component{Name: c.Name, Status: "up"}
			if c.Fn == nil {
				res[i].Status = "not_configured"
			} else if err := c.Fn(cctx); err != nil {
				res[i].Status, res[i].Detail = "down", "Not reachable."
			}
		}()
	}
	wg.Wait()
	out.Components = res
	return out, nil
}

func (s *Service) SetName(ctx context.Context, p *auth.Principal, name, ip, reqID string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return httpx.Validation(httpx.FieldError{Name: "platform_name", Message: "Enter a name of 1 to 60 characters."})
	}
	raw, _ := json.Marshal(name)
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO settings (tenant_id, key, value, updated_by) VALUES ($1,$2,$3,$4)
ON CONFLICT (tenant_id, key) DO UPDATE SET value=EXCLUDED.value, updated_by=EXCLUDED.updated_by, updated_at=now()`, p.TenantID, KeyPlatformName, raw, p.UserID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "settings.update", ResourceType: "settings", IP: ip, RequestID: reqID, Detail: map[string]any{"platform_name": name}})
	})
}

type Handlers struct{ Svc *Service }

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.Get(r.Context(), p)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, v)
	}))
	mux.HandleFunc("PATCH /api/v1/settings", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in struct {
			PlatformName string `json:"platform_name"`
		}
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err := h.Svc.SetName(r.Context(), p, in.PlatformName, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
