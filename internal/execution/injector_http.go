package execution

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"orchis-go/internal/hackbot"
	"orchis-go/internal/httpx"
)

// InjectorPrefix: jalur yang dilayani dengan token bearer (bukan cookie). CSRF tidak berlaku karena peramban
// tidak pernah mengirim header Authorization sendiri.
const InjectorPrefix = "/api/v1/injector/"

type InjectorHandlers struct{ RT *Runtime }

// Middleware memasang injector terautentikasi pada konteks. Cookie diabaikan sepenuhnya di jalur ini.
func (h *InjectorHandlers) guard(next func(w http.ResponseWriter, r *http.Request, in *Injector)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			httpx.Write(w, r, httpx.Unauthorized())
			return
		}
		in, err := h.RT.Authenticate(r.Context(), tok)
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		next(w, r, in)
	}
}

func (h *InjectorHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/recover-hackbot", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
		var res hackbot.Result
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&res); err != nil {
			httpx.Write(w, r, httpx.BadRequest("Invalid recovery result JSON."))
			return
		}
		if _, err = decoder.Token(); err != io.EOF {
			httpx.Write(w, r, httpx.BadRequest("Unexpected data after recovery result."))
			return
		}
		scan, err := h.RT.RecoverHackbot(context.WithoutCancel(r.Context()), in, id, res)
		reply(w, r, 200, map[string]uuid.UUID{"scan_id": scan}, err)
	}))
	mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/retention", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		retained, err := h.RT.RetainWorkspace(r.Context(), in, id)
		reply(w, r, 200, map[string]bool{"retained": retained}, err)
	}))
	mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/progress", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		var p hackbot.Progress
		if err = httpx.Decode(w, r, &p); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err = h.RT.Progress(r.Context(), in, id, p); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST "+InjectorPrefix+"heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		var body struct {
			Version string `json:"version"`
		}
		if err := httpx.Decode(w, r, &body); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err := h.RT.Beat(r.Context(), in, body.Version); err != nil {
			httpx.Write(w, r, err)
			return
		}
		httpx.JSON(w, 200, map[string]string{"integration": in.Key})
	}))
	mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/claim", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		spec, err := h.RT.Claim(r.Context(), in, id)
		reply(w, r, 200, spec, err)
	}))
	mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/heartbeat", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		cancel, err := h.RT.Heartbeat(r.Context(), in, id)
		reply(w, r, 200, map[string]bool{"cancel": cancel}, err)
	}))
	mux.HandleFunc("POST "+InjectorPrefix+"executions/{id}/result", h.guard(func(w http.ResponseWriter, r *http.Request, in *Injector) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Write(w, r, httpx.NotFound("Execution"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		var res Result
		if err := decodeResult(r, &res); err != nil {
			httpx.Write(w, r, err)
			return
		}
		if err := h.RT.Complete(context.WithoutCancel(r.Context()), in, id, res); err != nil {
			httpx.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func decodeResult(r *http.Request, res *Result) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(res); err != nil {
		return httpx.BadRequest("The result is not valid JSON: " + err.Error())
	}
	return nil
}
