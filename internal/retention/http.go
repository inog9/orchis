package retention

import (
	"github.com/google/uuid"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

func respond(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, 200, v)
}
func objectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Organisation or deletion"))
		return uuid.Nil, false
	}
	return id, true
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/retention", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, e := h.Svc.Get(r.Context(), p)
		respond(w, r, v, e)
	}))
	m.HandleFunc("PUT /api/v1/retention", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in Policy
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Update(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		respond(w, r, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations/{id}/deletion-preview", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, ok := objectID(w, r)
		if !ok {
			return
		}
		v, e := h.Svc.Preview(r.Context(), p, id)
		respond(w, r, v, e)
	}))
	m.HandleFunc("DELETE /api/v1/organizations/{id}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, ok := objectID(w, r)
		if !ok {
			return
		}
		var in DeleteInput
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.DeleteOrganization(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		respond(w, r, v, e)
	}))
	m.HandleFunc("GET /api/v1/data-deletions/{id}", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, ok := objectID(w, r)
		if !ok {
			return
		}
		v, e := h.Svc.Deletion(r.Context(), p, id)
		respond(w, r, v, e)
	}))
}
