package execution

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

// TriageInput menetapkan status penanganan satu finding. Risiko diterima wajib punya alasan tertulis
// dan tanggal habis; setelah tanggalnya lewat temuan otomatis kembali dihitung sebagai aksi.
type TriageInput struct {
	Status    string     `json:"status"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}

var triageStatuses = map[string]bool{"open": true, "accepted_risk": true, "false_positive": true, "fixed_pending": true}

// SetTriage mengubah status penanganan finding dan mencatat siapa/kapan di kolom dan di audit_log.
func (s *Service) SetTriage(ctx context.Context, p *auth.Principal, id uuid.UUID, in TriageInput, ip, req string) (*Finding, error) {
	if !authz.Has(p.Permissions, authz.FindingWrite) {
		return nil, httpx.Forbidden(authz.FindingWrite)
	}
	if !triageStatuses[in.Status] {
		return nil, httpx.Validation(httpx.FieldError{Name: "status", Message: "Choose open, accepted_risk, false_positive or fixed_pending."})
	}
	in.Reason = truncRunes(strings.TrimSpace(in.Reason), 1000)
	expires := in.ExpiresAt
	if in.Status == "accepted_risk" {
		if in.Reason == "" {
			return nil, httpx.Validation(httpx.FieldError{Name: "reason", Message: "An accepted risk needs a written reason."})
		}
		if expires == nil {
			return nil, httpx.Validation(httpx.FieldError{Name: "expires_at", Message: "An accepted risk needs an expiry date."})
		}
		if !expires.After(time.Now()) {
			return nil, httpx.Validation(httpx.FieldError{Name: "expires_at", Message: "The expiry date must be in the future."})
		}
	} else {
		expires = nil // hanya risiko diterima yang menyimpan tanggal habis
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var cmd interface{ RowsAffected() int64 }
		var err error
		if in.Status == "open" {
			cmd, err = tx.Exec(ctx, `UPDATE findings SET triage_status='open',triage_reason='',triage_expires_at=NULL,triage_by=NULL,triage_at=NULL WHERE tenant_id=$1 AND id=$2`, p.TenantID, id)
		} else {
			cmd, err = tx.Exec(ctx, `UPDATE findings SET triage_status=$3,triage_reason=$4,triage_expires_at=$5,triage_by=$6,triage_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, in.Status, in.Reason, expires, p.UserID)
		}
		if err != nil {
			return err
		}
		if cmd.RowsAffected() == 0 {
			return httpx.NotFound("Finding")
		}
		detail := map[string]any{"status": in.Status}
		if in.Reason != "" {
			detail["reason"] = truncRunes(in.Reason, 500)
		}
		if expires != nil {
			detail["expires_at"] = expires.UTC().Format(time.RFC3339)
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "finding.triage", ResourceType: "finding", ResourceID: &id, IP: ip, RequestID: req, Detail: detail})
	})
	if err != nil {
		return nil, err
	}
	return s.GetFinding(ctx, p, id)
}

func truncRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
