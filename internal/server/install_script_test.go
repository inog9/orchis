package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchis-go/internal/server"
)

func TestInstallScriptIsServedPublicallyAndIsValidShell(t *testing.T) {
	_, env := setup(t)
	srv := httptest.NewServer(server.New(server.Options{DB: env.App, Tenant: env.TenantA}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/x-shellscript") ||
		resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("headers", resp.StatusCode, resp.Header)
	}
	s := string(body)
	if !strings.HasPrefix(s, "#!/bin/sh\n") {
		t.Fatal("not a POSIX shell script")
	}
	for _, forbidden := range []string{"curl -k", "--insecure", " -k ", "--no-check-certificate"} {
		if strings.Contains(s, forbidden) {
			t.Fatal("installer must never disable TLS verification:", forbidden)
		}
	}
	for _, required := range []string{"openssl pkeyutl -verify", "SHA-256 mismatch", "--uninstall", "--pin", "not yet supported"} {
		if !strings.Contains(s, required) {
			t.Fatal("missing", required)
		}
	}
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", "-n", path).CombinedOutput(); err != nil {
		t.Fatal("sh -n:", string(out))
	}
	// Only GET is allowed.
	post, err := http.Post(srv.URL+"/install.sh", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode == 200 {
		t.Fatal("POST must not be served the script")
	}
}
