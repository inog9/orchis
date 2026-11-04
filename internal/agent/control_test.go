package agent

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/agentproto"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUninstallCommandBindingAndPrivateReceipt(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	command := uuid.NewString()
	a := &Agent{Cfg: &Config{AgentID: id, Secret: "private-fixture"}, ConfigPath: filepath.Join(dir, "agent-config.json"), ServiceManaged: true, StartUninstall: func() error { return nil }}
	for _, c := range []agentproto.ControlCommand{{ID: command, AgentID: uuid.NewString(), Kind: "uninstall"}, {ID: command, AgentID: id, Kind: "shell"}, {ID: "../bad", AgentID: id, Kind: "uninstall"}} {
		if e := a.beginUninstall(context.Background(), &c); e == nil {
			t.Fatal("unsafe command accepted", c)
		}
	}
	called := false
	a.StartUninstall = func() error {
		called = true
		c, e := LoadConfig(a.ConfigPath)
		if e != nil || c.Uninstall == nil || c.Uninstall.ID != command || !c.Uninstall.Managed {
			t.Fatal("helper started without durable receipt", c, e)
		}
		st, e := os.Stat(a.ConfigPath)
		if e != nil || st.Mode().Perm() != 0600 {
			t.Fatal("receipt is not private", e)
		}
		return nil
	}
	if e := a.beginUninstall(context.Background(), &agentproto.ControlCommand{ID: command, AgentID: id, Kind: "uninstall"}); e != nil || !called {
		t.Fatal("valid command did not start", e)
	}
}
func TestRemoveOwnedFilesPreservesSiblingInstanceAndSystemPackages(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("physical removal helper not supported on Windows")
	}
	dir := t.TempDir()
	own := filepath.Join(dir, "instance-a")
	other := filepath.Join(dir, "instance-b")
	_ = os.Mkdir(own, 0700)
	_ = os.Mkdir(other, 0700)
	a := &Agent{Exe: filepath.Join(own, "agent"), ConfigPath: filepath.Join(own, "agent-config.json")}
	inst := a.osqueryInstaller()
	_ = os.MkdirAll(filepath.Dir(inst.BinPath()), 0700)
	keep := []string{filepath.Join(other, "agent"), filepath.Join(other, "agent-config.json"), filepath.Join(own, "operator-note.txt"), filepath.Join(own, "osquery", "unrelated-file")}
	older := filepath.Join(own, "osquery", "1.2.3", filepath.Base(inst.BinPath()))
	_ = os.MkdirAll(filepath.Dir(older), 0700)
	remove := []string{a.Exe, a.Exe + ".old", a.ConfigPath, a.ConfigPath + ".tmp", inst.BinPath(), older}
	for _, file := range append(append([]string{}, keep...), remove...) {
		if e := os.WriteFile(file, []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := a.RemoveOwnedFiles(); e != nil {
		t.Fatal(e)
	}
	for _, file := range remove {
		if _, e := os.Stat(file); !os.IsNotExist(e) {
			t.Fatal("owned file remains", file, e)
		}
	}
	for _, file := range keep {
		if _, e := os.Stat(file); e != nil {
			t.Fatal("unrelated file removed", file, e)
		}
	}
}
func TestRemoveOwnedFilesRefusesSymlinkCacheAndForeignConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink test")
	}
	for _, unsafe := range []string{"cache", "config"} {
		t.Run(unsafe, func(t *testing.T) {
			dir := t.TempDir()
			external := t.TempDir()
			exe := filepath.Join(dir, "agent")
			_ = os.WriteFile(exe, []byte("binary"), 0700)
			a := &Agent{Exe: exe, ConfigPath: filepath.Join(dir, "agent-config.json")}
			if unsafe == "config" {
				a.ConfigPath = filepath.Join(external, "agent-config.json")
			} else if e := os.Symlink(external, filepath.Join(dir, "osquery")); e != nil {
				t.Fatal(e)
			}
			if e := a.RemoveOwnedFiles(); e == nil {
				t.Fatal("unsafe removal accepted")
			}
			if _, e := os.Stat(exe); e != nil {
				t.Fatal("binary removed on unsafe path", e)
			}
		})
	}
}
func TestClientPauseDoesNotClaimAndUninstallDoesNotAckBeforeRemoval(t *testing.T) {
	for _, mode := range []string{"paused", "uninstall"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var claims, acks atomic.Int32
			id := uuid.NewString()
			command := uuid.NewString()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/agent/heartbeat":
					w.Header().Set("Content-Type", "application/json")
					if mode == "paused" {
						_, _ = w.Write([]byte(`{"paused":true,"poll_seconds":2}`))
						go func() { time.Sleep(50 * time.Millisecond); cancel() }()
					} else {
						_, _ = w.Write([]byte(`{"paused":true,"command":{"id":"` + command + `","agent_id":"` + id + `","kind":"uninstall"}}`))
					}
				case "/api/v1/agent/control/ack":
					acks.Add(1)
					w.WriteHeader(204)
				case "/api/v1/agent/executions/claim":
					claims.Add(1)
					cancel()
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			a := &Agent{Cfg: &Config{AgentID: id, Secret: "private", ServerURL: srv.URL}, Exe: filepath.Join(t.TempDir(), "agent"), Version: "1.0.0"}
			a.ConfigPath = filepath.Join(filepath.Dir(a.Exe), "agent-config.json")
			var started bool
			a.StartUninstall = func() error { started = true; return nil }
			if e := a.Run(ctx); e != nil {
				t.Fatal(e)
			}
			if claims.Load() != 0 || acks.Load() != 0 {
				t.Fatal("paused/uninstall agent claimed or prematurely acknowledged", claims.Load(), acks.Load())
			}
			if mode == "uninstall" && !started {
				t.Fatal("uninstall helper not started")
			}
		})
	}
}
func TestUninstallHelperLaunchFailureIsReportedWithoutRawError(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 4096)
		n, _ := r.Body.Read(b)
		body = string(b[:n])
		w.WriteHeader(204)
	}))
	defer srv.Close()
	a := &Agent{Cfg: &Config{AgentID: uuid.NewString(), Secret: "private", ServerURL: srv.URL}, ConfigPath: filepath.Join(t.TempDir(), "agent-config.json"), StartUninstall: func() error { return errors.New("sensitive /root/path") }}
	if e := a.beginUninstall(context.Background(), &agentproto.ControlCommand{ID: uuid.NewString(), AgentID: a.Cfg.AgentID, Kind: "uninstall"}); e == nil {
		t.Fatal("launch failure not returned")
	}
	if strings.Contains(body, "sensitive") || strings.Contains(body, "/root/path") {
		t.Fatal("raw failure transmitted")
	}
	if body == "" || a.Cfg.Uninstall != nil {
		t.Fatal("launch failure not acknowledged", body)
	}
}
