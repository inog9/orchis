package reporting

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"strconv"
)

type Handlers struct{ Svc *Service }

const cols = `id,organization_id,assessment_id,version,status,error,created_at,snapshot`

func scanReport(row pgx.Row) (Report, error) {
	var r Report
	var data []byte
	e := row.Scan(&r.ID, &r.OrganizationID, &r.AssessmentID, &r.Version, &r.Status, &r.Error, &r.CreatedAt, &data)
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(data, &r.Snapshot)
	return r, e
}
func (s *Service) Get(ctx context.Context, tenant, id uuid.UUID) (Report, error) {
	var v Report
	e := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		var e error
		v, e = scanReport(tx.QueryRow(ctx, `SELECT `+cols+` FROM reports WHERE tenant_id=$1 AND id=$2`, tenant, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Report")
		}
		return e
	})
	return v, e
}

type handler func(http.ResponseWriter, *http.Request, *auth.Principal)

func access(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		for _, perm := range []string{authz.OrganizationRead, authz.AssessmentRead, authz.AssetRead, authz.FindingRead} {
			if !authz.Has(p.Permissions, perm) {
				httpx.Write(w, r, httpx.Forbidden(perm))
				return
			}
		}
		next(w, r, p)
	}
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/reports", auth.Require(authz.ReportRead, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var org, assessment, cur *uuid.UUID
		for key, dst := range map[string]**uuid.UUID{"organization_id": &org, "assessment_id": &assessment, "cursor": &cur} {
			raw := r.URL.Query().Get(key)
			if raw == "" {
				continue
			}
			if key == "cursor" {
				b, e := base64.RawURLEncoding.DecodeString(raw)
				if e != nil {
					httpx.Write(w, r, httpx.BadRequest("Invalid cursor."))
					return
				}
				raw = string(b)
			}
			id, e := uuid.Parse(raw)
			if e != nil {
				httpx.Write(w, r, httpx.BadRequest("Invalid filter."))
				return
			}
			*dst = &id
		}
		items := []Report{}
		var next *string
		e := h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			rows, e := tx.Query(r.Context(), `SELECT id,organization_id,assessment_id,version,status,error,created_at,snapshot-'assets' FROM reports WHERE tenant_id=$1 AND ($2::uuid IS NULL OR organization_id=$2) AND ($3::uuid IS NULL OR assessment_id=$3) AND ($4::uuid IS NULL OR id<$4) ORDER BY id DESC LIMIT 26`, p.TenantID, org, assessment, cur)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				v, e := scanReport(rows)
				if e != nil {
					return e
				}
				v.Snapshot.Assets = nil
				items = append(items, v)
			}
			return rows.Err()
		})
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		if len(items) > 25 {
			items = items[:25]
			v := base64.RawURLEncoding.EncodeToString([]byte(items[24].ID.String()))
			next = &v
		}
		httpx.JSON(w, 200, map[string]any{"items": items, "next_cursor": next})
	})))
	m.HandleFunc("GET /api/v1/reports/{id}", auth.Require(authz.ReportRead, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := uuid.Parse(r.PathValue("id"))
		if e != nil {
			httpx.Write(w, r, httpx.NotFound("Report"))
			return
		}
		v, e := h.Svc.Get(r.Context(), p.TenantID, id)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 200, v)
	})))
	m.HandleFunc("GET /api/v1/reports/{id}/export/{format}", auth.Require(authz.ReportRead, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := uuid.Parse(r.PathValue("id"))
		if e != nil {
			httpx.Write(w, r, httpx.NotFound("Report"))
			return
		}
		v, e := h.Svc.Get(r.Context(), p.TenantID, id)
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		var data []byte
		format := r.PathValue("format")
		content := ""
		switch format {
		case "json":
			content = "application/json"
			data, e = json.MarshalIndent(v.Snapshot, "", "  ")
		case "csv":
			content = "text/csv; charset=utf-8"
			data, e = CSV(v.Snapshot)
		case "html":
			content = "text/html; charset=utf-8"
			data, e = HTML(v.Snapshot)
		case "pdf":
			content = "application/pdf"
			if v.Status != "ready" {
				httpx.Write(w, r, httpx.Conflict("The PDF is not ready. HTML and JSON are available."))
				return
			}
			store, ok := h.Svc.Store.(interface {
				Get(context.Context, string) ([]byte, error)
			})
			if !ok {
				httpx.Write(w, r, httpx.NewProblem(503, "Report storage unavailable", ""))
				return
			}
			data, e = store.Get(r.Context(), p.TenantID.String()+"/reports/"+id.String()+".pdf")
		default:
			httpx.Write(w, r, httpx.NotFound("Export format"))
			return
		}
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		w.Header().Set("Content-Type", content)
		disposition := "attachment"
		if format == "html" && r.URL.Query().Get("preview") == "1" {
			disposition = "inline"
		}
		w.Header().Set("Content-Disposition", disposition+"; filename=\"orchis-report-"+id.String()+"."+format+"\"")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	})))
	m.HandleFunc("POST /api/v1/reports/{id}/retry", auth.Require(authz.ReportGenerate, access(func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, e := uuid.Parse(r.PathValue("id"))
		if e != nil {
			httpx.Write(w, r, httpx.NotFound("Report"))
			return
		}
		e = h.Svc.DB.InTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			tag, e := tx.Exec(r.Context(), `UPDATE reports SET status='queued',attempts=0,error='',lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='failed'`, p.TenantID, id)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return httpx.Conflict("Only failed PDFs can be retried.")
			}
			return audit.Write(r.Context(), tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "report.retry", ResourceType: "report", ResourceID: &id, IP: auth.ClientIP(r), RequestID: httpx.RequestIDFrom(r.Context())})
		})
		if e != nil {
			httpx.Write(w, r, e)
			return
		}
		httpx.JSON(w, 202, map[string]string{"status": "queued"})
	})))
}
