package recommendations

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
)

type Handlers struct {
	Svc     *Service
	Runtime *execution.Runtime
}

func (h *Handlers) worker(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			httpx.Write(w, r, httpx.Unauthorized())
			return
		}
		in, err := h.Runtime.Authenticate(r.Context(), token)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if in.Key != "hackbot" {
			httpx.Write(w, r, httpx.Forbidden("recommendation worker"))
			return
		}
		next(w, r)
	}
}
func (h *Handlers) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/scans/{id}/recommendations", auth.Require(authz.AssessmentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if !authz.Has(p.Permissions, authz.FindingRead) || !authz.Has(p.Permissions, authz.ReportRead) {
			httpx.Write(w, r, httpx.Forbidden(authz.ReportRead))
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Scan"))
			return
		}
		job, err := h.Svc.Get(r.Context(), p.TenantID, id)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, map[string]any{"job": job})
	}))
	m.HandleFunc("POST /api/v1/injector/recommendations/claim", h.worker(func(w http.ResponseWriter, r *http.Request) {
		claim, err := h.Svc.Claim(r.Context())
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, map[string]any{"claim": claim})
	}))
	m.HandleFunc("POST /api/v1/injector/recommendations/{id}/result", h.worker(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Review"))
			return
		}
		var c Completion
		if err = httpx.Decode(w, r, &c); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err = h.Svc.Complete(r.Context(), id, c); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(204)
	}))
}
