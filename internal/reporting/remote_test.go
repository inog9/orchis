package reporting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteRendererBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		ok   bool
	}{
		{"pdf", 200, "%PDF-test", true}, {"error", 500, "%PDF-test", false}, {"html", 200, "<html>", false}, {"redirect", 302, "%PDF-test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("x", 32) {
					t.Error("missing renderer authentication")
				}
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			s := Service{RendererURL: srv.URL, RendererToken: strings.Repeat("x", 32)}
			_, e := s.RenderPDF(context.Background(), []byte("safe"))
			if (e == nil) != tc.ok {
				t.Fatalf("error=%v", e)
			}
		})
	}
	s := Service{RendererURL: "http://invalid", RendererToken: "short"}
	if _, e := s.RenderPDF(context.Background(), nil); e == nil {
		t.Fatal("weak credential accepted")
	}
}
