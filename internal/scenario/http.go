package scenario

import (
	"net/http"

	"github.com/google/uuid"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

func id(r *http.Request) (uuid.UUID, error) {
	v, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return uuid.Nil, httpx.NotFound("Scenario")
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

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/scenarios", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.List(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/scenarios", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in Input
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("GET /api/v1/scenarios/{id}", auth.Require(authz.ScenarioRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		sid, err := id(r)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Get(r.Context(), p, sid)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("PATCH /api/v1/scenarios/{id}", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		sid, err := id(r)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in Patch
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Patch(r.Context(), p, sid, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("DELETE /api/v1/scenarios/{id}", auth.Require(authz.ScenarioDelete, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		sid, err := id(r)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err := h.Svc.Delete(r.Context(), p, sid, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("PUT /api/v1/scenarios/{id}/targets", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		sid, err := id(r)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in TargetsInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.SetTargets(r.Context(), p, sid, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("PUT /api/v1/scenarios/{id}/graph", auth.Require(authz.ScenarioWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		sid, err := id(r)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in GraphInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.SetGraph(r.Context(), p, sid, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
}
