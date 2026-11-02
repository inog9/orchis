package ai

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"html/template"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"os"
	"strings"
)

type Handlers struct {
	Svc     *Service
	Reports *reporting.Service
}
type endpoint func(http.ResponseWriter, *http.Request, *auth.Principal)

func access(perm string, f endpoint) http.HandlerFunc {
	return auth.Require(perm, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		for _, v := range []string{authz.OrganizationRead, authz.ReportRead, authz.AssessmentRead, authz.FindingRead, authz.AssetRead} {
			if !authz.Has(p.Permissions, v) {
				httpx.Write(w, r, httpx.Forbidden(v))
				return
			}
		}
		f(w, r, p)
	})
}
func parsed(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, e := uuid.Parse(r.PathValue(key))
	if e != nil {
		httpx.Write(w, r, httpx.NotFound("Resource"))
		return id, false
	}
	return id, true
}
func reply(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	httpx.JSON(w, 200, v)
}
func (h *Handlers) Register(mux *http.ServeMux) {
	s := h.Svc
	mux.HandleFunc("GET /api/v1/organizations/{id}/ai", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		v, e := s.Policy(r.Context(), p, org)
		reply(w, r, v, e)
	}))
	mux.HandleFunc("PUT /api/v1/organizations/{id}/ai", access("ai.manage", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		var in PolicyInput
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := s.SavePolicy(r.Context(), p, org, in)
		reply(w, r, v, e)
	}))
	mux.HandleFunc("GET /api/v1/organizations/{id}/ai/preview", access("ai.generate", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		report, e := uuid.Parse(r.URL.Query().Get("report_id"))
		if e != nil {
			httpx.Write(w, r, httpx.BadRequest("Choose a report."))
			return
		}
		v, _, e := s.Preview(r.Context(), p, org, report)
		reply(w, r, v, e)
	}))
	mux.HandleFunc("GET /api/v1/organizations/{id}/ai/jobs", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		v, e := s.List(r.Context(), p, org, r.URL.Query().Get("cursor"))
		next := ""
		if len(v) == 50 {
			next = v[49].ID.String()
		}
		reply(w, r, map[string]any{"items": v, "next_cursor": next}, e)
	}))
	mux.HandleFunc("POST /api/v1/organizations/{id}/ai/jobs", access("ai.generate", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		org, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		var in Request
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := s.Create(r.Context(), p, org, in, r.Header.Get("Idempotency-Key"))
		reply(w, r, v, e)
	}))
	mux.HandleFunc("GET /api/v1/ai/jobs/{id}", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		v, e := s.Get(r.Context(), p, id)
		reply(w, r, v, e)
	}))
	for _, op := range []string{"cancel", "approve"} {
		op := op
		perm := "ai.generate"
		if op == "approve" {
			perm = "ai.approve"
		}
		mux.HandleFunc("POST /api/v1/ai/jobs/{id}/"+op, access(perm, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
			id, ok := parsed(w, r, "id")
			if !ok {
				return
			}
			var in struct {
				Version int `json:"version"`
			}
			if e := httpx.Decode(w, r, &in); e != nil {
				httpx.Write(w, r, e)
				return
			}
			var v Job
			var e error
			if op == "approve" {
				v, e = s.Approve(r.Context(), p, id, in.Version)
			} else {
				v, e = s.Cancel(r.Context(), p, id, in.Version)
			}
			reply(w, r, v, e)
		}))
	}
	mux.HandleFunc("GET /api/v1/ai/templates", access("ai.read", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		list := []map[string]any{}
		e := s.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			rows, e := tx.Query(r.Context(), `SELECT id,kind,version,instructions,is_demo FROM ai_templates WHERE tenant_id=$1 ORDER BY kind,version DESC LIMIT 100`, p.TenantID)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				var kind, text string
				var v int
				var synthetic bool
				if e = rows.Scan(&id, &kind, &v, &text, &synthetic); e != nil {
					return e
				}
				list = append(list, map[string]any{"id": id, "kind": kind, "version": v, "instructions": text, "is_demo": synthetic})
			}
			return rows.Err()
		})
		reply(w, r, map[string]any{"items": list, "builtin_instructions": Instructions, "builtin_version": PromptVersion}, e)
	}))
	mux.HandleFunc("POST /api/v1/ai/templates", access("ai.manage", func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in struct {
			Kind         string `json:"kind"`
			Instructions string `json:"instructions"`
		}
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		if (in.Kind != "planning" && in.Kind != "remediation") || !validText(in.Instructions, 6000) {
			httpx.Write(w, r, httpx.BadRequest("Invalid template purpose or instructions."))
			return
		}
		id := ids.New()
		version := 0
		e := s.DB.InTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			// Penguncian tenant menserialkan versi template tanpa sequence global.
			if _, e := tx.Exec(r.Context(), `SELECT id FROM tenants WHERE id=$1 FOR UPDATE`, p.TenantID); e != nil {
				return e
			}
			if e := tx.QueryRow(r.Context(), `SELECT COALESCE(max(version),0)+1 FROM ai_templates WHERE tenant_id=$1 AND kind=$2`, p.TenantID, in.Kind).Scan(&version); e != nil {
				return e
			}
			_, e := tx.Exec(r.Context(), `INSERT INTO ai_templates(id,tenant_id,kind,version,instructions,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, p.TenantID, in.Kind, version, in.Instructions, p.UserID)
			if e != nil {
				return e
			}
			return note(r.Context(), tx, p, uuid.Nil, id, "ai.template", map[string]any{"kind": in.Kind, "version": version})
		})
		reply(w, r, map[string]any{"id": id, "version": version}, e)
	}))
	mux.HandleFunc("GET /api/v1/ai/jobs/{id}/export/{format}", access("ai.read", h.export))
	worker := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			v := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(s.WorkerToken) < 32 || subtle.ConstantTimeCompare([]byte(v), []byte(s.WorkerToken)) != 1 {
				httpx.Write(w, r, httpx.Unauthorized())
				return
			}
			f(w, r)
		}
	}
	mux.HandleFunc("POST /api/v1/ai-worker/claim", worker(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Claim(r.Context())
		if e == nil && v == nil {
			w.WriteHeader(204)
			return
		}
		reply(w, r, v, e)
	}))
	mux.HandleFunc("POST /api/v1/ai-worker/jobs/{id}/check", worker(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, e := s.Alive(r.Context(), id, in.Token)
		reply(w, r, map[string]bool{"active": v}, e)
	}))
	mux.HandleFunc("POST /api/v1/ai-worker/jobs/{id}/complete", worker(func(w http.ResponseWriter, r *http.Request) {
		id, ok := parsed(w, r, "id")
		if !ok {
			return
		}
		var in Completion
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		if e := s.Complete(r.Context(), id, in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		w.WriteHeader(204)
	}))
}
func (h *Handlers) export(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, ok := parsed(w, r, "id")
	if !ok {
		return
	}
	switch r.PathValue("format") {
	case "json", "pdf":
	default:
		httpx.Write(w, r, httpx.NotFound("Export format"))
		return
	}
	j, e := h.Svc.Get(r.Context(), p, id)
	if e != nil {
		httpx.Write(w, r, e)
		return
	}
	if j.Output == nil {
		httpx.Write(w, r, httpx.Conflict("AI output is not available."))
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ai-%s.%s"`, id, r.PathValue("format")))
	switch r.PathValue("format") {
	case "json":
		httpx.JSON(w, 200, j)
	case "pdf":
		var html bytes.Buffer
		if e := adviceTemplate.Execute(&html, j); e != nil {
			httpx.Write(w, r, e)
			return
		}
		renderer := h.Reports
		if renderer == nil {
			renderer = &reporting.Service{Script: os.Getenv("ORCHIS_REPORT_RENDER_SCRIPT")}
		}
		pdf, e := renderer.RenderPDF(r.Context(), html.Bytes())
		if e != nil {
			httpx.Write(w, r, httpx.NewProblem(503, "PDF unavailable", "The AI advice is saved. JSON remains available."))
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	default:
		httpx.Write(w, r, httpx.NotFound("Export format"))
	}
}

var adviceTemplate = template.Must(template.New("advice").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="orchis-document-type" content="ai-advice"><title>AI review</title><style>@page{size:A4;margin:18mm 16mm 22mm}body{font:14px sans-serif;max-width:800px;margin:40px auto;padding:20px;overflow-wrap:anywhere}@media print{body{margin:0;padding:0;max-width:none}}article{break-inside:avoid;border-top:1px solid;padding:16px 0}h1,h2{break-after:avoid}.provenance{break-inside:avoid}pre{font-size:12px;line-height:1.35}</style>{{if .IsDemo}}<p class="provenance"><strong>DEMO / SYNTHETIC - local fake API output. No external AI model or check was invoked.</strong></p>{{end}}<h1>AI advisory supplement</h1><p>Unverified AI guidance. Review before applying. The original assessment snapshot is unchanged.</p><p>Source report: {{.ReportID}}</p><p>Job {{.ID}} · {{.Provider}} · {{.Model}} · prompt {{.PromptVersion}}</p><p>{{.Output.Summary}}</p>{{range .Output.Remediations}}<article><h2>{{index $.References.FindingLabels .FindingID}}</h2><p>Asset: {{index $.References.FindingTargets .FindingID}} | {{index $.References.AssetLabels (index $.References.FindingTargets .FindingID)}}</p><p>Reference: {{.FindingID}} | Source finding: {{index $.References.Findings .FindingID}}</p><p>{{.Recommendation}}</p><p>Validation: {{.Validation}}</p></article>{{end}}<section class="provenance"><h2>Recorded instructions</h2><pre style="white-space:pre-wrap;overflow-wrap:anywhere">{{.Instructions}}</pre></section></html>`))
