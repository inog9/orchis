package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
)

// AgentPrefix: jalur yang dilayani dengan kredensial agent (bukan cookie). Dikecualikan dari aturan CSRF karena tidak pernah memakai cookie.
const AgentPrefix = "/api/v1/agent/"

type Handlers struct {
	Svc *Service
	RT  *execution.Runtime
}

func reply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}

func bearer(r *http.Request) string {
	t, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return t
}

func (h *Handlers) guard(next func(w http.ResponseWriter, r *http.Request, p *Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.Svc.Authenticate(r.Context(), bearer(r))
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		next(w, r, p)
	}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	h.registerControls(mux)
	// ---- konsol (cookie/token API pengguna) ----
	mux.HandleFunc("GET /api/v1/agents", auth.Require(authz.AgentRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.List(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/agents/enrollment-tokens", auth.Require(authz.AgentManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in TokenInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.CreateEnrollmentToken(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("GET /api/v1/agents/enrollment-tokens/{id}", auth.Require(authz.AgentManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Enrollment token"))
			return
		}
		v, err := h.Svc.EnrollmentTokenStatus(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("POST /api/v1/agents/{id}/revoke", auth.Require(authz.AgentManage, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Agent"))
			return
		}
		if err := h.Svc.Revoke(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// ---- agent ----
	mux.HandleFunc("POST "+AgentPrefix+"enroll", func(w http.ResponseWriter, r *http.Request) {
		var in EnrollInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Enroll(r.Context(), in, auth.ClientIP(r))
		reply(w, r, 200, v, err)
	})
	mux.HandleFunc("POST "+AgentPrefix+"heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		var in HeartbeatInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Heartbeat(r.Context(), p, in, auth.ClientIP(r))
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("POST "+AgentPrefix+"executions/claim", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		spec, err := h.RT.ClaimNext(r.Context(), p.Worker())
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if spec == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httpx.JSON(w, 200, spec)
	}))
	mux.HandleFunc("POST "+AgentPrefix+"executions/{id}/claim", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		var permitted bool
		err = h.Svc.DB.InTenantRead(r.Context(), h.Svc.Tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM endpoint_collection_requests WHERE tenant_id=$1 AND execution_id=$2 AND agent_id=$3)`, h.Svc.Tenant, id, p.AgentID).Scan(&permitted)
		})
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if !permitted {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		spec, err := h.RT.Claim(r.Context(), p.Worker(), id)
		reply(w, r, 200, spec, err)
	}))
	mux.HandleFunc("POST "+AgentPrefix+"executions/{id}/heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		cancel, err := h.RT.Heartbeat(r.Context(), p.Worker(), id)
		reply(w, r, 200, map[string]bool{"cancel": cancel}, err)
	}))
	mux.HandleFunc("POST "+AgentPrefix+"executions/{id}/result", h.guard(func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		var res execution.Result
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&res); err != nil {
			httpx.Write(w, r, httpx.BadRequest("The result is not valid JSON: "+err.Error()))
			return
		}
		if err := h.RT.Complete(context.WithoutCancel(r.Context()), p.Worker(), id, res); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET "+AgentPrefix+"download", h.download)
	mux.HandleFunc("GET "+AgentPrefix+"tool", h.guard(h.tool))
}

// download menyajikan biner rilis untuk platform milik kredensial: agent yang sedang memperbarui diri (oag_) atau pemasang yang
// memegang token pendaftaran yang belum terpakai (oen_). Platform diambil dari kredensial, bukan dari parameter.
func (h *Handlers) download(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	var osName, arch string
	switch {
	case strings.HasPrefix(tok, secretPrefix):
		p, err := h.Svc.Authenticate(r.Context(), tok)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		osName, arch = p.OS, p.Arch
	case strings.HasPrefix(tok, tokenPrefix) && len(tok) < 200:
		err := h.Svc.DB.InTenantRead(r.Context(), h.Svc.Tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(r.Context(), `SELECT os, arch FROM agent_enrollment_tokens WHERE tenant_id=$1 AND token_hash=$2 AND used_at IS NULL AND expires_at > $3`,
				h.Svc.Tenant, auth.HashToken(tok), h.Svc.now()).Scan(&osName, &arch)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Write(w, r, httpx.Unauthorized())
			return
		}
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
	default:
		httpx.Write(w, r, httpx.Unauthorized())
		return
	}
	m, err := h.Svc.Release.Manifest()
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Agent release"))
		return
	}
	if want := r.URL.Query().Get("version"); want != "" && want != m.Version {
		httpx.Write(w, r, httpx.Conflict("The server now offers version "+m.Version+", not "+want+"."))
		return
	}
	t, ok := m.Find(osName, arch)
	if !ok {
		httpx.Write(w, r, httpx.NotFound("Agent build for this platform"))
		return
	}
	f, err := os.Open(h.Svc.Release.Path(t))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Release-Version", m.Version)
	w.Header().Set("X-Sha256", t.SHA256)
	w.Header().Set("X-Signature-Ed25519", t.Signature)
	w.Header().Set("Content-Disposition", `attachment; filename="`+t.File+`"`)
	http.ServeContent(w, r, t.File, time.Time{}, f)
}
