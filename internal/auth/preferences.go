package auth

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"orchis-go/internal/audit"
	"orchis-go/internal/httpx"
)

// SetLanguage changes only the authenticated account. Empty means browser default.
func (s *Service) SetLanguage(ctx context.Context, p *Principal, language, ip string) error {
	if language != "" && language != "en" && language != "id" {
		return httpx.Validation(httpx.FieldError{Name: "ui_language", Message: "Choose English or Indonesia."})
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET ui_language=NULLIF($3,'') WHERE tenant_id=$1 AND id=$2 AND status='active' AND deleted_at IS NULL AND NOT is_demo`, p.TenantID, p.UserID, language)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return httpx.Unauthorized()
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "user.language", ResourceType: "user", ResourceID: &p.UserID, IP: ip, RequestID: httpx.RequestIDFrom(ctx), Detail: map[string]any{"ui_language": language}})
	})
}
func (h *Handlers) Preferences(w http.ResponseWriter, r *http.Request, p *Principal) {
	var in struct {
		Language string `json:"ui_language"`
	}
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	if err := h.Svc.SetLanguage(r.Context(), p, in.Language, ClientIP(r)); err != nil {
		httpx.Write(w, r, err)
		return
	}
	out := *p
	out.UILanguage = in.Language
	httpx.JSON(w, http.StatusOK, h.toDTO(&out))
}
