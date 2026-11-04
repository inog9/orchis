package organizations

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"net/http"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/scancontrol"
	"strconv"
	"time"
)

type Handlers struct{ Svc *Service }

func reply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}
func parseID(r *http.Request) (uuid.UUID, error) {
	id, e := uuid.Parse(r.PathValue("id"))
	if e != nil {
		return id, httpx.NotFound("Organization or assessment")
	}
	return id, nil
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/organizations/{id}/readiness", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Readiness(r.Context(), p, id)
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		limit := 25
		if v := r.URL.Query().Get("limit"); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 {
				httpx.Write(w, r, httpx.BadRequest("Invalid limit."))
				return
			}
			limit = n
		}
		v, e := h.Svc.List(r.Context(), p, limit, r.URL.Query().Get("cursor"))
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations/{id}", auth.Require(authz.OrganizationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Get(r.Context(), p, id)
		reply(w, r, 200, v, e)
	}))
	for _, method := range []string{"POST", "PATCH"} {
		method := method
		path := "/api/v1/organizations"
		if method == "PATCH" {
			path += "/{id}"
		}
		m.HandleFunc(method+" "+path, auth.Require(authz.OrganizationWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
			var in Input
			if e := httpx.Decode(w, r, &in); e != nil {
				httpx.Write(w, r, e)
				return
			}
			id := uuid.Nil
			if method == "PATCH" {
				var e error
				id, e = parseID(r)
				if e != nil {
					httpx.Write(w, r, e)
					return
				}
			}
			v, e := h.Svc.Save(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
			status := 200
			if method == "POST" {
				status = 201
			}
			reply(w, r, status, v, e)
		}))
	}
	m.HandleFunc("PUT /api/v1/organizations/{id}/assets", auth.Require(authz.OrganizationWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var in MembershipInput
		if e = httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		if e = h.Svc.Members(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); e != nil {
			httpx.Write(w, r, e)
			return
		}
		w.WriteHeader(204)
	}))
	m.HandleFunc("POST /api/v1/organizations/{id}/assessments", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var in AssessmentInput
		if e = httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Launch(r.Context(), p, id, in, r.Header.Get("Idempotency-Key"), auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 202, v, e)
	}))
	m.HandleFunc("GET /api/v1/organizations/{id}/assessments", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Assessments(r.Context(), p, id, r.URL.Query().Get("cursor"))
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("GET /api/v1/assessments/{id}", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := h.Svc.Assessment(r.Context(), p, id)
		reply(w, r, 200, v, e)
	}))
	m.HandleFunc("POST /api/v1/assessments/{id}/cancel", auth.Require(authz.AssessmentRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := parseID(r)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		e = h.Svc.Cancel(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, map[string]bool{"canceled": e == nil}, e)
	}))
}
func (s *Service) Cancel(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, req string) error {
	if e := permitted(p, authz.SimulationRun); e != nil {
		return e
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		tag, e := tx.Exec(ctx, `UPDATE assessments SET status='canceled',ended_at=now() WHERE tenant_id=$1 AND id=$2 AND status='running'`, p.TenantID, id)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return httpx.Conflict("This assessment is not running or does not exist.")
		}
		if _, e = tx.Exec(ctx, `UPDATE simulations SET status='canceled',ended_at=now(),cancel_reason='Assessment canceled',version=version+1,updated_at=now() WHERE tenant_id=$1 AND id IN (SELECT simulation_id FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=$2) AND status IN ('running','paused')`, p.TenantID, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE executions SET state='ended',outcome='canceled',ended_at=now(),lease_until=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND simulation_id IN(SELECT simulation_id FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=$2) AND state IN('ready','running')`, p.TenantID, id); e != nil {
			return e
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "assessment.cancel", ResourceType: "assessment", ResourceID: &id, IP: ip, RequestID: req})
	})
}
func (s *Service) RunTicker(ctx context.Context, tenant uuid.UUID) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		if err := s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error { return s.Refresh(ctx, tx, tenant) }); err != nil && ctx.Err() == nil {
			slog.Error("assessment refresh gagal", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
