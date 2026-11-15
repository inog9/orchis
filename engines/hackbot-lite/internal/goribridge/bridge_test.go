package goribridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(id int64, url string) Flow {
	text := "console.log('hello');"
	return Flow{
		Flow:     Row{ID: id, URL: url, Method: "GET", Status: 200, ContentType: "application/javascript"},
		Request:  Message{Head: "GET /assets/app.js HTTP/1.1\r\nHost: app.test:8443\r\nCookie: actor=test\r\n\r\n"},
		Response: Message{Head: "HTTP/1.1 200 OK\r\nContent-Type: application/javascript\r\nContent-Encoding: gzip\r\nTransfer-Encoding: chunked\r\nTrailer: X-Checksum\r\n\r\n", Body: &Body{Encoding: "text", Text: text, Size: int64(len(text)), Note: "dechunked · decoded: gzip"}},
	}
}

func TestMapPreservesOriginAndDecodesWireHeaders(t *testing.T) {
	f := fixture(42, "https://app.test:8443/assets/app.js?v=2")
	p, digest, err := MapFlow(f)
	if err != nil {
		t.Fatal(err)
	}
	if p.RequestURL != f.Flow.URL || !strings.Contains(p.Request, "Cookie: actor=test") {
		t.Fatal("lost URL port/query or identity headers")
	}
	for _, encoded := range []string{"Content-Encoding:", "Transfer-Encoding:", "Trailer:"} {
		if strings.Contains(p.Response, encoded) {
			t.Fatalf("stale wire header %s", encoded)
		}
	}
	if !strings.HasSuffix(p.Response, "\r\n\r\n"+f.Response.Body.Text) || !strings.Contains(p.Response, fmt.Sprintf("Content-Length: %d", len(f.Response.Body.Text))) {
		t.Fatal("body framing mismatch")
	}
	if len(digest) != 64 {
		t.Fatal("missing dedup digest")
	}
	f.Flow.URL = "https://other.test/app.js"
	_, other, _ := MapFlow(f)
	if other == digest {
		t.Fatal("dedup collapsed different URLs")
	}
}

func TestIncompleteAndUnsupportedBodiesAreNeverIngested(t *testing.T) {
	for _, kind := range []string{"truncated", "wire", "decode", "omitted", "missing", "size", "unsupported", "base64"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(1, "https://app.test/app.js")
			switch kind {
			case "truncated":
				f.Response.Body.Truncated = true
			case "wire":
				f.Response.Body.WireTruncated = true
			case "decode":
				f.Response.Body.DecodeTruncated = true
			case "omitted":
				f.Response.Body.Omitted = true
			case "missing":
				f.Response.Body = nil
			case "size":
				f.Response.Body.Size++
			case "unsupported":
				f.Response.Body.Note = "compressed: zstd (unsupported)"
			case "base64":
				f.Response.Body.Encoding = "base64"
				f.Response.Body.Base64 = "invalid!"
			}
			if _, _, err := MapFlow(f); err == nil {
				t.Fatal("accepted invalid capture")
			}
		})
	}
}

func TestBase64AndPlainEmptyBodies(t *testing.T) {
	f := fixture(1, "https://app.test/app.js")
	f.Response.Head = "HTTP/1.1 200 OK\r\nContent-Type: application/javascript\r\n\r\n"
	f.Response.Body.Encoding = "base64"
	f.Response.Body.Base64 = base64.StdEncoding.EncodeToString([]byte(f.Response.Body.Text))
	if _, _, err := MapFlow(f); err != nil {
		t.Fatal(err)
	}
	f.Response.Body = &Body{Encoding: "text", Size: 0}
	if _, _, err := MapFlow(f); err != nil {
		t.Fatal(err)
	}
}

func TestScopeExactOriginsAndExpiry(t *testing.T) {
	s := &Scope{Origins: []string{"https://app.test:8443", "https://default.test"}, Methods: []string{"GET"}}
	for _, tc := range []struct {
		url, method string
		want        bool
	}{
		{"https://app.test:8443/a.js", "GET", true},
		{"https://app.test/a.js", "GET", false},
		{"https://evil.app.test:8443/a.js", "GET", false},
		{"https://app.test:8443/a.js", "POST", false},
		{"https://default.test:443/a.js", "GET", true},
		{"https://user@app.test:8443/a.js", "GET", false},
	} {
		if got := s.Allows(Row{URL: tc.url, Method: tc.method}); got != tc.want {
			t.Errorf("%s %s = %v", tc.method, tc.url, got)
		}
	}
	expired := time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	s.ExpiresAt = &expired
	if s.Active(time.Now()) {
		t.Fatal("expired scope active")
	}
}

func testBridge(t *testing.T, flows []Flow, handler http.HandlerFunc) (*Bridge, Config, *int) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	c := Config{DB: filepath.Join(dir, "gori.db"), Events: filepath.Join(dir, "capture.jsonl"), State: filepath.Join(dir, "checkpoint.json"), ScopeFile: filepath.Join(dir, "state.json"), IngestURL: server.URL, Once: true, Logger: log.New(io.Discard, "", 0)}
	writeScope(t, c.ScopeFile, "https://app.test:8443")
	if err := os.WriteFile(c.Events, nil, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	b.run = func(_ context.Context, args ...string) ([]byte, error) {
		runs++
		if args[1] == "history" {
			if strings.Contains(strings.Join(args, " "), "--count") {
				return json.Marshal(map[string]int{"count": len(flows)})
			}
			var all []byte
			for _, f := range flows {
				data, _ := json.Marshal(f.Flow)
				all = append(all, data...)
				all = append(all, '\n')
			}
			return all, nil
		}
		for _, f := range flows {
			if args[2] == fmt.Sprint(f.Flow.ID) {
				if !strings.Contains(" "+strings.Join(args, " ")+" ", " --no-redact ") {
					t.Fatal("private ingestion export must explicitly disable configured Gori redaction")
				}
				return json.Marshal(f)
			}
		}
		return nil, fmt.Errorf("unknown flow")
	}
	return b, c, &runs
}

func writeScope(t *testing.T, path string, origins ...string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"mode": "running", "scope": Scope{Origins: origins, Methods: []string{"GET"}}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryDedupAndRestart(t *testing.T) {
	calls := 0
	flows := []Flow{fixture(1, "https://app.test:8443/app.js"), fixture(2, "https://app.test:8443/app.js"), fixture(3, "https://out.test/app.js")}
	b, c, _ := testBridge(t, flows, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var got Ingestion
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		if got.RequestURL != "https://app.test:8443/app.js" {
			t.Errorf("wrong asset %s", got.RequestURL)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	if err := b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || b.checkpoint.Flows["2"] != "duplicate_asset" || b.checkpoint.Flows["3"] != "out_of_scope" {
		t.Fatalf("calls=%d checkpoint=%+v", calls, b.checkpoint)
	}
	info, err := os.Stat(c.State)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("checkpoint is not private")
	}
	restarted, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	restarted.run = b.run
	if err = restarted.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("restart redelivered persisted asset")
	}
}

func TestIngestionOutageDoesNotLoseEvents(t *testing.T) {
	calls := 0
	f := fixture(7, "https://app.test:8443/app.js")
	b, _, _ := testBridge(t, nil, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(202)
		}
	})
	b.run = func(_ context.Context, args ...string) ([]byte, error) { return json.Marshal(f) }
	_, _ = b.loadScope()
	line, _ := json.Marshal(f.Flow)
	if err := os.WriteFile(b.config.Events, append(line, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.events(context.Background()); err == nil {
		t.Fatal("outage ignored")
	}
	if b.checkpoint.Offset != 0 || b.checkpoint.Flows["7"] != "" {
		t.Fatal("failed delivery checkpointed")
	}
	if err := b.events(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || b.checkpoint.Offset == 0 {
		t.Fatal("event was not retried")
	}
}

func TestPartialEventWaitsAndScopeChangeReopensRecovery(t *testing.T) {
	f := fixture(9, "https://app.test:8443/a.js")
	calls := 0
	b, c, _ := testBridge(t, []Flow{f}, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(202) })
	writeScope(t, c.ScopeFile, "https://other.test")
	if err := b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("outside-scope ingestion")
	}
	writeScope(t, c.ScopeFile, "https://app.test:8443")
	if err := b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("scope expansion did not reprocess skipped history")
	}
	line, _ := json.Marshal(f.Flow)
	if err := os.WriteFile(c.Events, line, 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.events(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.checkpoint.Offset != 0 {
		t.Fatal("partial event consumed")
	}
	fh, err := os.OpenFile(c.Events, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fh.WriteString("\n")
	fh.Close()
	if err = b.events(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.checkpoint.Offset != int64(len(line)+1) {
		t.Fatal("completed line not consumed")
	}
}

func TestRecoveryBoundAndMissingScopeFailClosed(t *testing.T) {
	b, c, runs := testBridge(t, []Flow{fixture(1, "https://app.test:8443/a.js"), fixture(2, "https://app.test:8443/b.js")}, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected ingestion") })
	if err := os.WriteFile(c.ScopeFile, []byte(`{"scope":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *runs != 0 {
		t.Fatal("ran Gori without scope")
	}
	writeScope(t, c.ScopeFile, "https://app.test:8443")
	b.config.HistoryLimit = 1
	if err := b.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "history-limit") {
		t.Fatalf("silent history truncation: %v", err)
	}
}

func TestPauseDuringExportDefersDeliveryAndOffset(t *testing.T) {
	f := fixture(17, "https://app.test:8443/app.js")
	calls := 0
	b, c, _ := testBridge(t, nil, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(202) })
	_, _ = b.loadScope()
	b.run = func(_ context.Context, args ...string) ([]byte, error) {
		if err := os.WriteFile(c.ScopeFile, []byte(`{"mode":"paused","scope":{"origins":["https://app.test:8443"],"methods":["GET"]}}`), 0600); err != nil {
			t.Fatal(err)
		}
		return json.Marshal(f)
	}
	line, _ := json.Marshal(f.Flow)
	if err := os.WriteFile(c.Events, append(line, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.events(context.Background()); err == nil {
		t.Fatal("pause during export was ignored")
	}
	if calls != 0 || b.checkpoint.Offset != 0 || b.checkpoint.Flows["17"] != "" {
		t.Fatal("paused delivery was consumed")
	}
	if err := b.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("paused run ingested traffic")
	}
	writeScope(t, c.ScopeFile, "https://app.test:8443")
	_, _ = b.loadScope()
	b.run = func(_ context.Context, args ...string) ([]byte, error) { return json.Marshal(f) }
	if err := b.events(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("resume did not retry deferred event")
	}
}
