package agent

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHTTPClientTrustsOnlyConfiguredCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHTTPClient("", time.Second); err != nil {
		t.Fatal(err)
	}
	plain, _ := NewHTTPClient("", 2*time.Second)
	if _, err := plain.Get(srv.URL); err == nil {
		t.Fatal("self-signed server must be refused without its CA")
	}
	c, err := NewHTTPClient(ca, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil || resp.StatusCode != 204 {
		t.Fatal("configured CA must be trusted", err)
	}
	resp.Body.Close()
}

func TestHTTPClientFailsClosedForUnusableCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	bad := filepath.Join(t.TempDir(), "bad.pem")
	_ = os.WriteFile(bad, []byte("not a certificate"), 0o600)
	if _, err := NewHTTPClient(bad, time.Second); err == nil {
		t.Fatal("garbage CA must be rejected")
	}
	if _, err := clientFor(filepath.Join(t.TempDir(), "missing.pem"), time.Second).Get(srv.URL); err == nil {
		t.Fatal("missing CA must not fall back to default trust")
	}
}
