package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

const CookieName = "orchis_session"

type ctxKey int

const principalKey ctxKey = 1

func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// Handlers menyajikan endpoint autentikasi. MVP memakai satu tenant, ditetapkan saat server naik.
type Handlers struct {
	Svc              *Service
	Tenant           uuid.UUID
	CookieSecure     bool
	AdvancedFeatures bool
}

func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return remoteIP(r)
}

// Middleware membaca cookie sesi. Permintaan tanpa sesi valid diteruskan tanpa Principal; Require yang menolaknya.
func (h *Handlers) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bila ada header Authorization, hanya itu yang dipakai (cookie diabaikan): tidak ada kebingungan siapa yang sedang bertindak.
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && strings.HasPrefix(tok, APITokenPrefix) {
			if p, err := h.Svc.AuthenticateToken(r.Context(), h.Tenant, tok); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), principalKey, p))
			} else if pr, isP := err.(*httpx.Problem); !isP || pr.Status != http.StatusUnauthorized {
				httpx.Write(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
			p, err := h.Svc.Authenticate(r.Context(), c.Value)
			if err == nil {
				r = r.WithContext(context.WithValue(r.Context(), principalKey, p))
			} else if pr, ok := err.(*httpx.Problem); !ok || pr.Status != http.StatusUnauthorized {
				httpx.Write(w, r, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Require: 401 bila belum masuk, 403 bila tidak punya permission yang diminta (perm kosong = cukup login).
func Require(perm string, next func(w http.ResponseWriter, r *http.Request, p *Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFrom(r.Context())
		if p == nil {
			httpx.Write(w, r, httpx.Unauthorized())
			return
		}
		if perm != "" && !authz.Has(p.Permissions, perm) {
			httpx.Write(w, r, httpx.Forbidden(perm))
			return
		}
		next(w, r, p)
	}
}

type userDTO struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"display_name"`
	Role        string          `json:"role"`
	Permissions []string        `json:"permissions"`
	Features    map[string]bool `json:"features"`
}

func (h *Handlers) toDTO(p *Principal) userDTO {
	return userDTO{ID: p.UserID.String(), Email: p.Email, DisplayName: p.DisplayName, Role: p.RoleKey, Permissions: p.Permissions, Features: map[string]bool{"advanced": h.AdvancedFeatures}}
}

func (h *Handlers) setCookie(w http.ResponseWriter, token string, remember bool) {
	c := &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode}
	if remember {
		c.MaxAge = int(RememberTTL / time.Second)
	}
	http.SetCookie(w, c)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	res, err := h.Svc.Login(r.Context(), h.Tenant, in.Email, in.Password, ClientIP(r), r.UserAgent(), in.RememberMe)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	h.setCookie(w, res.Token, res.Remember)
	httpx.JSON(w, http.StatusOK, h.toDTO(res.Principal))
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request, p *Principal) {
	if err := h.Svc.Logout(r.Context(), p, ClientIP(r)); err != nil {
		httpx.Write(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request, p *Principal) {
	httpx.JSON(w, http.StatusOK, h.toDTO(p))
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", Require("", h.Logout))
	mux.HandleFunc("GET /api/v1/auth/me", Require("", h.Me))
}
