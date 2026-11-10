package schedules

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/scancontrol"
)

type Handlers struct{ Svc *Service }

func reply(w http.ResponseWriter, r *http.Request, status int, v any, e error) {
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, status, v)
}
func path(r *http.Request) (uuid.UUID, error) {
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		return id, httpx.NotFound("Schedule")
	}
	return id, nil
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/audit-schedules/preview", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if e := access(p, false); e != nil {
			httpx.Write(w, r, e)
			return
		}
		var rule Rule
		if e := httpx.Decode(w, r, &rule); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := rule.Preview(h.Svc.now())
		if e != nil {
			e = validation("rule", e.Error())
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		raw, e := httpx.OrganizationParam(r.URL.Query(), false)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		restricted := []bool{}
		if raw != "" {
			org := uuid.MustParse(raw)
			e = h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
				var exists bool
				if e := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations WHERE tenant_id=$1 AND id=$2)`, p.TenantID, org).Scan(&exists); e != nil {
					return e
				}
				if !exists {
					return httpx.NotFound("Organisation")
				}
				for _, at := range v {
					state, e := scancontrol.Read(r.Context(), tx, p.TenantID, &org)
					if e != nil {
						return e
					}
					allowed, e := state.Window.Allows(at)
					if e != nil {
						return e
					}
					restricted = append(restricted, state.GlobalPaused || state.Paused || !allowed)
				}
				return nil
			})
		}
		reply(w, r, 200, map[string]any{"items": v, "restricted": restricted}, e)
	}))
	m.HandleFunc("GET /api/v1/audit-schedules", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		raw, e := httpx.OrganizationParam(r.URL.Query(), false)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var org *uuid.UUID
		if raw != "" {
			v := uuid.MustParse(raw)
			org = &v
		}
		v, next, e := h.Svc.List(r.Context(), p, org, r.URL.Query().Get("cursor"))
		reply(w, r, 200, map[string]any{"items": v, "next_cursor": next}, e)
	}))
	m.HandleFunc("POST /api/v1/audit-schedules", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in Input
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Save(r.Context(), p, uuid.Nil, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, e)
	}))
	m.HandleFunc("GET /api/v1/audit-schedules/{id}", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := path(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Get(r.Context(), p, id)
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("PUT /api/v1/audit-schedules/{id}", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := path(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var in Input
		if e = httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Save(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("POST /api/v1/audit-schedules/{id}/pause", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := path(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var in struct {
			Paused  bool `json:"paused"`
			Version int  `json:"version"`
		}
		if e = httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Pause(r.Context(), p, id, in.Paused, in.Version, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("DELETE /api/v1/audit-schedules/{id}", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := path(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var in struct {
			Version int `json:"version"`
		}
		if e = httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		e = h.Svc.Delete(r.Context(), p, id, in.Version, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		w.WriteHeader(204)
	}))
	m.HandleFunc("GET /api/v1/audit-schedules/{id}/runs", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := path(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, next, e := h.Svc.History(r.Context(), p, id, r.URL.Query().Get("cursor"))
		reply(w, r, 200, map[string]any{"items": v, "next_cursor": next}, e)
	}))
}
