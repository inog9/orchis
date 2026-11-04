package scancontrol

import (
	"github.com/google/uuid"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

func (h *Handlers) handler(action string, global bool) func(http.ResponseWriter, *http.Request, *auth.Principal) {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org *uuid.UUID
		if !global {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil {
				httpx.Write(w, r, httpx.NotFound("Organisation"))
				return
			}
			org = &id
		}
		var state State
		var e error
		if action == "get" {
			state, e = h.Svc.Get(r.Context(), p, org)
		} else {
			var in Input
			if action != "stop" {
				if e = httpx.Decode(w, r, &in); e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			state, e = h.Svc.Change(r.Context(), p, org, action, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, state)
	}
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/scan-control", auth.Require(authz.SettingsManage, h.handler("get", true)))
	m.HandleFunc("POST /api/v1/scan-control/stop", auth.Require(authz.SettingsManage, h.handler("stop", true)))
	m.HandleFunc("POST /api/v1/scan-control/resume", auth.Require(authz.SettingsManage, h.handler("resume", true)))
	m.HandleFunc("GET /api/v1/organizations/{id}/scan-control", auth.Require(authz.OrganizationRead, h.handler("get", false)))
	m.HandleFunc("PUT /api/v1/organizations/{id}/scan-control", auth.Require(authz.SettingsManage, h.handler("window", false)))
	m.HandleFunc("POST /api/v1/organizations/{id}/scan-control/stop", auth.Require(authz.SettingsManage, h.handler("stop", false)))
	m.HandleFunc("POST /api/v1/organizations/{id}/scan-control/resume", auth.Require(authz.SettingsManage, h.handler("resume", false)))
}
