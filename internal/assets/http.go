package assets

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

func (h *Handlers) List(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "limit", Message: "Must be a positive integer."}))
			return
		}
		limit = n
	}
	page, err := h.Svc.List(r.Context(), p, ListFilter{Organization: q.Get("organization_id"), Category: q.Get("category"), Query: q.Get("q"), Limit: limit, Cursor: q.Get("cursor")})
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Asset"))
		return
	}
	a, err := h.Svc.Get(r.Context(), p, id)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in CreateInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	a, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/assets/"+a.ID.String())
	httpx.JSON(w, http.StatusCreated, a)
}

func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Asset"))
		return
	}
	var in PatchInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	a, err := h.Svc.Patch(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Asset"))
		return
	}
	if err := h.Svc.Delete(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
		httpx.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/assets", auth.Require(authz.AssetRead, h.List))
	mux.HandleFunc("GET /api/v1/assets/{id}", auth.Require(authz.AssetRead, h.Get))
	mux.HandleFunc("POST /api/v1/assets", auth.Require(authz.AssetWrite, h.Create))
	mux.HandleFunc("POST /api/v1/assets/import/preview", auth.Require(authz.AssetWrite, h.ImportPreview))
	mux.HandleFunc("POST /api/v1/assets/import", auth.Require(authz.AssetWrite, h.Import))
	mux.HandleFunc("PATCH /api/v1/assets/{id}", auth.Require(authz.AssetWrite, h.Patch))
	mux.HandleFunc("DELETE /api/v1/assets/{id}", auth.Require(authz.AssetDelete, h.Delete))
}
