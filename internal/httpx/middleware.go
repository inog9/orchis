package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = 1

func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }

// Base: request id, pemulihan panic (menjadi 500 dengan error_id), dan log akses.
func Base(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := "req_" + strings.TrimPrefix(newErrorID(), "err_")
		r = r.WithContext(WithRequestID(r.Context(), id))
		w.Header().Set("X-Request-Id", id)
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "request_id", id, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				Write(sw, r, fmt.Errorf("panic: %v", rec))
			}
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds(), "request_id", id)
		}()
		next.ServeHTTP(sw, r)
	})
}

// CSRF: permintaan yang mengubah data wajib membawa header khusus. Peramban tidak dapat menambahkannya
// pada permintaan lintas-situs tanpa preflight CORS, sehingga ini melengkapi cookie SameSite.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			// Jalur injector memakai token bearer (bukan cookie); peramban tidak dapat memalsukan header Authorization lintas situs.
			if strings.HasPrefix(r.URL.Path, "/api/v1/injector/") && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				break
			}
			// Jalur agent: kredensial agent/token pendaftaran di header atau badan, tidak pernah cookie.
			if strings.HasPrefix(r.URL.Path, "/api/v1/agent/") {
				break
			}
			// Token API (oat_...): dipakai skrip, bukan peramban; header Authorization tidak dapat dipalsukan lintas situs.
			if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer oat_") {
				break
			}
			if r.Header.Get("X-Requested-With") != "orchis-web" {
				Write(w, r, NewProblem(403, "Request rejected", "The X-Requested-With header is required."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Decode membaca JSON dengan batas ukuran dan menolak kolom tak dikenal.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return BadRequest("The request body is not valid JSON: " + err.Error())
	}
	if _, err := dec.Token(); err != io.EOF {
		return BadRequest("The request body has extra data after the JSON object.")
	}
	return nil
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
