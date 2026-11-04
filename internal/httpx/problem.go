// Package httpx berisi utilitas HTTP: respons kesalahan seragam (RFC 7807), middleware, dan JSON.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type FieldError struct {
	Code    string `json:"code,omitempty"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type Problem struct {
	Code      string       `json:"code"`
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	ErrorID   string       `json:"error_id"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
	// RetryAfter dalam detik (dipakai untuk akun terkunci).
	RetryAfter int `json:"retry_after_seconds,omitempty"`
}

func (p *Problem) Error() string { return p.Title + ": " + p.Detail }

func newErrorID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "err_" + hex.EncodeToString(b)
}

func NewProblem(status int, title, detail string) *Problem {
	return &Problem{Code: problemCode(status, detail), Type: "about:blank", Title: title, Status: status, Detail: detail, ErrorID: newErrorID()}
}

func BadRequest(detail string) *Problem { return NewProblem(400, "Invalid request", detail) }
func Unauthorized() *Problem {
	return NewProblem(401, "Not signed in", "No session, or the session has ended.")
}
func Forbidden(perm string) *Problem {
	return NewProblem(403, "Forbidden", "Required permission: "+perm)
}
func NotFound(what string) *Problem {
	return NewProblem(404, "Not found", what+" not found.")
}
func Conflict(detail string) *Problem { return NewProblem(409, "Conflict", detail) }
func Validation(f ...FieldError) *Problem {
	p := NewProblem(422, "Invalid data", "Check the highlighted fields.")
	for i := range f {
		if f[i].Code == "" {
			f[i].Code = knownMessageCode(f[i].Message)
			if f[i].Code == "" {
				f[i].Code = "field_invalid"
			}
		}
	}
	p.Fields = f
	return p
}

// Write menulis Problem; kesalahan lain dijadikan 500 generik (detail hanya di log, tidak ke klien).
func Write(w http.ResponseWriter, r *http.Request, err error) {
	var p *Problem
	if !errors.As(err, &p) {
		p = NewProblem(500, "Something went wrong on our side", "Unexpected error. Quote the error_id if it persists.")
		slog.Error("kesalahan internal", "error_id", p.ErrorID, "request_id", RequestIDFrom(r.Context()), "err", err)
	}
	p.RequestID = RequestIDFrom(r.Context())
	if p.RetryAfter > 0 {
		w.Header().Set("Retry-After", itoa(p.RetryAfter))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	json.NewEncoder(w).Encode(p)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
