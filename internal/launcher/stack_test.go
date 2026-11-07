package launcher

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGoriConfigReplacesOnlyLegacyProxies(t *testing.T) {
	mcp := map[string]any{"mcpServers": map[string]any{"burpsuite": true, "Caido": true, "textbee": true, "custom": true}}
	root := t.TempDir()
	configureGori(mcp, root)
	servers := mcp["mcpServers"].(map[string]any)
	if len(servers) != 3 || servers["custom"] != true || servers["textbee"] != true {
		t.Fatalf("unexpected servers: %v", servers)
	}
	gori := servers["gori"].(map[string]any)
	args := gori["args"].([]string)
	if args[0] != "mcp" || args[1] != "--db" || args[2] != filepath.Join(root, "private", "stack", "gori.db") {
		t.Fatal(args)
	}
}

func TestStackReadinessAndFailure(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	for _, tc := range []struct{ name, script, want string }{
		{"ready", "import pathlib,sys,json,time\np=pathlib.Path(sys.argv[1])/'private/stack/ready.json'\np.parent.mkdir(parents=True,exist_ok=True)\np.write_text(json.dumps({'ready':True}))\ntime.sleep(30)\n", ""},
		{"failure", "import sys\nsys.exit(7)\n", "exited before readiness"},
		{"timeout", "import time\ntime.sleep(30)\n", "timed out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			script := filepath.Join(root, "supervisor.py")
			if err := os.WriteFile(script, []byte(tc.script), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := startStack(script, root, io.Discard, time.Second)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			s.stop()
			s.stop() // idempotent cleanup
			select {
			case <-s.done:
			default:
				t.Fatal("supervisor survived cleanup")
			}
		})
	}
}
