package scans

import (
	"github.com/google/uuid"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

func reply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}
func pathID(r *http.Request, key string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		return uuid.Nil, httpx.NotFound("Scan or network")
	}
	return id, nil
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/scans", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in Input
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, e := h.Svc.Launch(r.Context(), p, in, r.Header.Get("Idempotency-Key"), auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, e)
	}))
	m.HandleFunc("GET /api/v1/scans", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
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
	m.HandleFunc("GET /api/v1/scans/{id}", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := pathID(r, "id")
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Get(r.Context(), p, id)
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("POST /api/v1/scans/{id}/cancel", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := pathID(r, "id")
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		e = h.Svc.Cancel(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, map[string]bool{"canceled": e == nil}, e)
	}))
	m.HandleFunc("POST /api/v1/scan-jobs/{id}/asset", auth.Require(authz.AssetWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := pathID(r, "id")
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.AddDiscoveredAsset(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, map[string]uuid.UUID{"id": v}, e)
	}))
	for _, method := range []string{"GET", "PUT"} {
		m.HandleFunc(method+" /api/v1/organizations/{org}/agents/{agent}/networks", auth.Require(authz.AgentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
			org, e := pathID(r, "org")
			if e != nil {
				httpx.Write(w, r, e)
				return
			}
			agent, e := pathID(r, "agent")
			if e != nil {
				httpx.Write(w, r, e)
				return
			}
			if r.Method == "GET" {
				v, e := h.Svc.Networks(r.Context(), p, org, agent)
				reply(w, r, 200, v, e)
				return
			}
			var in ApproveInput
			if e = httpx.Decode(w, r, &in); e != nil {
				httpx.Write(w, r, e)
				return
			}
			v, e := h.Svc.AuthorizeRange(r.Context(), p, org, agent, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
			reply(w, r, 200, v, e)
		}))
	}
}
