package demo

import (
	"net/http"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

// Register expose hanya status (semua pengguna login) dan purge (settings.manage). Seed TIDAK
// diekspos lewat HTTP (hanya CLI `server demo-seed`): tidak ada endpoint seed publik/tak terotorisasi.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/demo/status", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		st, err := h.Svc.Status(r.Context(), p.TenantID)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, st)
	}))
	mux.HandleFunc("POST /api/v1/demo/purge", auth.Require(authz.SettingsManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		actor := p.UserID
		st, err := h.Svc.Purge(r.Context(), p.TenantID, &actor, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, st)
	}))
}
