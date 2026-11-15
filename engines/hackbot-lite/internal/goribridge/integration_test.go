package goribridge

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Opt in with a pinned Gori v0.9.1 binary. This sends only to local fixtures.
func TestGoriIntegration(t *testing.T) {
	binary := os.Getenv("GORI_TEST_BINARY")
	if binary == "" {
		t.Skip("set GORI_TEST_BINARY to run actual Gori CLI/capture integration")
	}
	const script = "export const lazy = () => import('./chunk.js');"
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = io.WriteString(gz, script)
		_ = gz.Close()
	}))
	defer target.Close()
	requests := 0
	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var payload Ingestion
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.RequestURL != target.URL+"/app.js" || !strings.HasSuffix(payload.Response, "\r\n\r\n"+script) || strings.Contains(payload.Response, "Content-Encoding:") {
			t.Errorf("unexpected mapped capture: %+v", payload)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingest.Close()
	dir := t.TempDir()
	config := Config{DB: filepath.Join(dir, "gori.db"), Events: filepath.Join(dir, "events.jsonl"), State: filepath.Join(dir, "checkpoint.json"), ScopeFile: filepath.Join(dir, "state.json"), IngestURL: ingest.URL, Gori: binary, Once: true, Logger: log.New(io.Discard, "", 0)}
	writeScope(t, config.ScopeFile, target.URL)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	events, err := os.Create(config.Events)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	diagnostics, err := os.Create(filepath.Join(dir, "gori.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer diagnostics.Close()
	cmd := exec.Command(binary, "run", "capture", "--db", config.DB, "--listen", "127.0.0.1", "--port", strconv.Itoa(port), "--format", "jsonl", "--ca-dir", filepath.Join(dir, "ca"))
	cmd.Env = append(os.Environ(), "GORI_HOME="+filepath.Join(dir, "home"))
	cmd.Stdout = events
	cmd.Stderr = diagnostics
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Signal(syscall.SIGTERM); _ = cmd.Wait() }()
	proxy, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableCompression: true}}
	deadline := time.Now().Add(15 * time.Second)
	for {
		conn, connectErr := net.DialTimeout("tcp", proxy.Host, 100*time.Millisecond)
		if connectErr == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			data, _ := os.ReadFile(diagnostics.Name())
			t.Fatalf("Gori startup failed: %s", data)
		}
		time.Sleep(50 * time.Millisecond)
	}
	response, err := client.Get(target.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	for {
		info, _ := events.Stat()
		if info.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Gori emitted no completion event")
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("want one mapped Gori capture, got %d", requests)
	}
	if err = b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("duplicate ingestion after event/history replay")
	}
}
