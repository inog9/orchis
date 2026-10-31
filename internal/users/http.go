package users

import (
	"net/http"

	"github.com/google/uuid"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct{ Svc *Service }

func uid(r *http.Request, what string) (uuid.UUID, error) {
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

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var in T
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return in, false
	}
	return in, true
}

func (h *Handlers) Register(mux *http.ServeMux) {
	// administrasi akun
	mux.HandleFunc("GET /api/v1/users", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.List(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("GET /api/v1/roles", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.Roles(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/users", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		in, ok := decode[CreateInput](w, r)
		if !ok {
			return
		}
		v, err := h.Svc.Create(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("PATCH /api/v1/users/{id}", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uid(r, "User")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		in, ok := decode[PatchInput](w, r)
		if !ok {
			return
		}
		v, err := h.Svc.Patch(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("POST /api/v1/users/{id}/reset-link", auth.Require(authz.UserManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uid(r, "User")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.CreateResetLink(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, err)
	}))

	// publik: lupa password dan pemakaian link reset
	mux.HandleFunc("POST /api/v1/auth/forgot", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[struct {
			Email string `json:"email"`
		}](w, r)
		if !ok {
			return
		}
		if err := h.Svc.Forgot(r.Context(), in.Email); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusAccepted) // sama untuk email yang ada maupun tidak
	})
	mux.HandleFunc("POST /api/v1/auth/reset/check", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[struct {
			Token string `json:"token"`
		}](w, r)
		if !ok {
			return
		}
		if err := h.Svc.CheckLink(r.Context(), in.Token); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/auth/reset", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}](w, r)
		if !ok {
			return
		}
		if err := h.Svc.UseLink(r.Context(), in.Token, in.Password, auth.ClientIP(r)); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// profil
	mux.HandleFunc("PATCH /api/v1/me", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		in, ok := decode[struct {
			DisplayName string `json:"display_name"`
		}](w, r)
		if !ok {
			return
		}
		if err := h.Svc.UpdateProfile(r.Context(), p, in.DisplayName); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/v1/me/password", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		in, ok := decode[struct {
			Current string `json:"current_password"`
			New     string `json:"new_password"`
		}](w, r)
		if !ok {
			return
		}
		if err := h.Svc.ChangePassword(r.Context(), p, in.Current, in.New, auth.ClientIP(r)); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/v1/me/tokens", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.Tokens(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/me/tokens", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		in, ok := decode[struct {
			Name          string `json:"name"`
			ExpiresInDays int    `json:"expires_in_days"`
		}](w, r)
		if !ok {
			return
		}
		v, err := h.Svc.CreateToken(r.Context(), p, in.Name, in.ExpiresInDays, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("DELETE /api/v1/me/tokens/{id}", auth.Require("", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uid(r, "Token")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err := h.Svc.RevokeToken(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
