package simulation

import (
	"net/http"

	"github.com/google/uuid"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

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

var statuses = map[string]bool{"draft": true, "scheduled": true, "running": true, "paused": true, "finished": true, "canceled": true, "failed": true}

func (h *Handlers) Register(mux *http.ServeMux) {
	h.registerDryRun(mux)
	mux.HandleFunc("POST /api/v1/scenarios/{id}/launch", auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Scenario")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in LaunchInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Launch(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 202, v, err)
	}))
	mux.HandleFunc("GET /api/v1/simulations", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		st := r.URL.Query().Get("status")
		if st != "" && !statuses[st] {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "status", Message: "Unknown status."}))
			return
		}
		v, err := h.Svc.List(r.Context(), p, ListFilter{Status: st})
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("GET /api/v1/simulations/{id}", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Simulation")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Get(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("GET /api/v1/simulations/{id}/attack-path", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Simulation")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.AttackPath(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	act := func(name string, f func(r *http.Request, p *auth.Principal, id uuid.UUID) (*Detail, error)) {
		mux.HandleFunc("POST /api/v1/simulations/{id}/"+name, auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
			id, err := pid(r, "Simulation")
			if err != nil {
				httpx.Write(w, r, err)
				return
			}
			v, err := f(r, p, id)
			reply(w, r, 200, v, err)
		}))
	}
	act("pause", func(r *http.Request, p *auth.Principal, id uuid.UUID) (*Detail, error) {
		return h.Svc.Pause(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	})
	act("resume", func(r *http.Request, p *auth.Principal, id uuid.UUID) (*Detail, error) {
		return h.Svc.Resume(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	})
	mux.HandleFunc("POST /api/v1/simulations/{id}/cancel", auth.Require(authz.SimulationRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pid(r, "Simulation")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in CancelInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Cancel(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
}
