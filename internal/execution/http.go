package execution

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
)

type Handlers struct {
	Svc    *Service
	Search Searcher
}

func pathID(r *http.Request, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return uuid.Nil, httpx.NotFound(what)
	}
	return id, nil
}

func reply[T any](w http.ResponseWriter, r *http.Request, status int, v T, err error) {
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}

func (h *Handlers) Register(mux *http.ServeMux) {
	h.registerPresets(mux)
	mux.HandleFunc("GET /api/v1/contracts", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.Contracts(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("GET /api/v1/atomic-tests", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		v, err := h.Svc.ListTests(r.Context(), p)
		reply(w, r, 200, map[string]any{"items": v}, err)
	}))
	mux.HandleFunc("POST /api/v1/atomic-tests", auth.Require(authz.AtomicWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		var in TestInput
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.CreateTest(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 201, v, err)
	}))
	mux.HandleFunc("GET /api/v1/atomic-tests/{id}", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Atomic test")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.GetTest(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("PATCH /api/v1/atomic-tests/{id}", auth.Require(authz.AtomicWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Atomic test")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in TestPatch
		if err := httpx.Decode(w, r, &in); err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.PatchTest(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("DELETE /api/v1/atomic-tests/{id}", auth.Require(authz.AtomicDelete, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Atomic test")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err := h.Svc.DeleteTest(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context())); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/v1/atomic-tests/{id}/run", auth.Require(authz.AtomicRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Atomic test")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Run(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 202, v, err)
	}))
	mux.HandleFunc("GET /api/v1/executions/{id}", auth.Require(authz.AtomicRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Execution")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.GetExecution(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("POST /api/v1/executions/{id}/cancel", auth.Require(authz.AtomicRun, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Execution")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.Cancel(r.Context(), p, id, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("GET /api/v1/findings", auth.Require(authz.FindingRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		q := r.URL.Query()
		organization, err := httpx.OrganizationParam(q, false)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		f := FindingFilter{Type: q.Get("type"), Severity: q.Get("severity"), Query: q.Get("q"), Cursor: q.Get("cursor"), Triage: q.Get("triage")}
		if organization != "" {
			id := uuid.MustParse(organization)
			f.OrganizationID = &id
		}
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "limit", Message: "Must be a positive integer."}))
				return
			}
			f.Limit = n
		}
		if v := q.Get("asset_id"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "asset_id", Message: "Invalid asset id."}))
				return
			}
			f.AssetID = &id
		}
		if v := q.Get("simulation_id"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				httpx.Write(w, r, httpx.Validation(httpx.FieldError{Name: "simulation_id", Message: "Invalid simulation id."}))
				return
			}
			f.SimulationID = &id
		}
		v, err := h.Svc.ListFindings(r.Context(), p, f, h.Search)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("GET /api/v1/findings/{id}", auth.Require(authz.FindingRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Finding")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		v, err := h.Svc.GetFinding(r.Context(), p, id)
		reply(w, r, 200, v, err)
	}))
	mux.HandleFunc("PUT /api/v1/findings/{id}/triage", auth.Require(authz.FindingWrite, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		id, err := pathID(r, "Finding")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		var in TriageInput
		if e := httpx.Decode(w, r, &in); e != nil {
			httpx.Write(w, r, e)
			return
		}
		v, err := h.Svc.SetTriage(r.Context(), p, id, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
		reply(w, r, 200, v, err)
	}))
}
