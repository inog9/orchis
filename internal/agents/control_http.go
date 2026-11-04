package agents

import (
	"github.com/google/uuid"
	"net/http"
	"orchis-go/internal/agentproto"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

func (h *Handlers) registerControls(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agents/rollout", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, e := h.Svc.Rollout(r.Context(), p)
		reply(w, r, 200, v, e)
	}))
	mux.HandleFunc("PUT /api/v1/agents/rollout", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in RolloutPolicy
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.SetRollout(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, e)
	}))
	mux.HandleFunc("POST /api/v1/agents/{id}/control", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := uuid.Parse(r.PathValue("id"))
		if e != nil {
			httpx.Write(w, r, httpx.NotFound("Agent"))
			return
		}
		var in ControlInput
		if e = httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Control(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, e)
	}))
	mux.HandleFunc("POST /api/v1/agents/{id}/collect", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if e := adminControl(p); e != nil {
			httpx.Write(w, r, e)
			return
		}
		id, e := uuid.Parse(r.PathValue("id"))
		if e != nil {
			httpx.Write(w, r, httpx.NotFound("Agent"))
			return
		}
		if h.Svc.CollectNow == nil {
			httpx.Write(w, r, httpx.Conflict("Endpoint collection is unavailable."))
			return
		}
		v, e := h.Svc.CollectNow(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 202, map[string]any{"execution_ids": v}, e)
	}))
	mux.HandleFunc("POST "+AgentPrefix+"control/ack", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		var in agentproto.ControlAck
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		if e := h.Svc.Ack(r.Context(), p, in, auth.ClientIP(r)); e != nil {
			httpx.Write(w, r, e)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
