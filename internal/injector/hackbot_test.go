package injector

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/hackbot"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHackbotWorkerLifecycle(t *testing.T) {
	for _, event := range []string{"complete", "limited", "input_required", "failed", "cancelled", "missing", "invalid"} {
		t.Run(event, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("ORCHIS_HACKBOT_DATA", root)
			reviewed := event == "complete" || event == "limited" || event == "input_required"
			result := hackbot.Result{Schema: 1, ReviewCompleted: reviewed, SessionID: "fixture", Disposition: event, Phase: "report", Summary: "Tested fixture coverage.", Report: "# Report", Validation: "# Ledger"}
			payload, _ := json.Marshal(result)
			code := map[string]int{"complete": 0, "limited": 2, "input_required": 3, "failed": 1, "cancelled": 130}[event]
			script := fmt.Sprintf(`#!/usr/bin/env python3
import sys,json,os
args=sys.argv
root=args[args.index('--dir')+1]
assert 'ORCHIS_INJECTOR_TOKEN' not in os.environ
prompt=sys.stdin.read()
assert prompt == 'Run pentest this website http://127.0.0.1, use skill pentest.'
if '--inputs-dir' in args:
 folder=args[args.index('--inputs-dir')+1]
 assert json.load(open(os.path.join(folder,'nmap.json')))['tool']=='nmap'
 assert json.load(open(os.path.join(folder,'nuclei.json')))['tool']=='nuclei'
file=os.path.join(root,'result.json')
open(file,'w').write(%q)
event=%q
if event=='invalid':
 print('not-json',flush=True)
elif event!='missing':
 print(json.dumps({'schema':1,'event':'started','sessionId':'fixture'}),flush=True)
 print(json.dumps({'schema':1,'event':event,'sessionId':'fixture','resultFile':file}),flush=True)
sys.exit(%d)
`, string(payload), event, code)
			bin := filepath.Join(root, "job")
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if filepath.Base(r.URL.Path) == "heartbeat" {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"cancel":false}`))
				} else {
					w.WriteHeader(204)
				}
			}))
			defer srv.Close()
			t.Setenv("ORCHIS_INJECTOR_TOKEN", "do-not-inherit")
			worker := Worker{Kind: "hackbot", Binary: bin, Client: NewClient(srv.URL, "fixture")}
			res := worker.execute(context.Background(), &execution.ClaimSpec{ExecutionID: uuid.New(), ContractKey: contract.KeyHackbot, ContractVersion: 1, Target: "http://127.0.0.1", Params: json.RawMessage(`{"language":"en"}`)})
			// Guard prevents loopback unless explicitly enabled.
			if res.Hackbot != nil {
				t.Fatal("scope guard was bypassed")
			}
			worker.Guard.AllowLoopback = true
			seeds := []hackbot.ScannerInput{{Tool: "nmap", Status: "finished", Target: "127.0.0.1", Params: json.RawMessage(`{}`), Findings: json.RawMessage(`[{"type":"open_port","value":"80/tcp"}]`)}, {Tool: "nuclei", Status: "finished", Target: "http://127.0.0.1", Params: json.RawMessage(`{}`), Findings: json.RawMessage(`[]`)}}
			res = worker.execute(context.Background(), &execution.ClaimSpec{ScannerInputs: seeds, ExecutionID: uuid.New(), ContractKey: contract.KeyHackbot, ContractVersion: 1, Target: "http://127.0.0.1", Params: json.RawMessage(`{"language":"en"}`)})
			if event == "missing" || event == "invalid" {
				if res.Error == "" || res.Hackbot != nil {
					t.Fatal("invalid job was accepted")
				}
				return
			}
			if res.Hackbot == nil {
				t.Fatalf("missing result: %s", res.Error)
			}
			if res.Hackbot.ReviewCompleted != reviewed {
				t.Fatal("completed review marker was lost")
			}
			if !strings.Contains(res.Hackbot.Summary, "nmap finished (1 observations)") {
				t.Fatal("summary lost preliminary scanner work")
			}
			if event == "complete" && res.Error != "" {
				t.Fatal(res.Error)
			}
			if event != "complete" && res.Error == "" {
				t.Fatal("incomplete job reported success")
			}
		})
	}
}
func TestHackbotArtifactBoundary(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "result.json")
	os.WriteFile(outside, []byte(`{}`), 0600)
	if _, err := readHackbotResult(root, outside); err == nil {
		t.Fatal("outside file accepted")
	}
	link := filepath.Join(root, "result.json")
	os.Symlink(outside, link)
	if _, err := readHackbotResult(root, link); err == nil {
		t.Fatal("symlink escape accepted")
	}
}

func TestHackbotWorkspaceCleanup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ORCHIS_HACKBOT_DATA", root)
	deleted, retained := uuid.NewString(), uuid.NewString()
	for _, name := range []string{deleted, retained, "operator-files"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keep := !strings.Contains(r.URL.Path, deleted)
		fmt.Fprintf(w, `{"retained":%t}`, keep)
	}))
	defer srv.Close()
	w := Worker{Kind: "hackbot", Client: NewClient(srv.URL, "fixture")}
	w.pruneHackbot(context.Background())
	if _, err := os.Stat(filepath.Join(root, deleted)); !os.IsNotExist(err) {
		t.Fatal("expired workspace retained")
	}
	for _, name := range []string{retained, "operator-files"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal("retained workspace deleted")
		}
	}
}

func TestScannerHandoffFiles(t *testing.T) {
	root := t.TempDir()
	inputs := []hackbot.ScannerInput{{Tool: "nmap", Status: "finished", Target: "127.0.0.1", Params: json.RawMessage(`{}`), Findings: json.RawMessage(`[{"type":"open_port","value":"80/tcp"}]`)}, {Tool: "nuclei", Status: "failed", Target: "http://127.0.0.1", Error: "fixture scan failed", Params: json.RawMessage(`{}`), Findings: json.RawMessage(`[]`)}}
	folder, err := writeScannerInputs(root, inputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"nmap", "nuclei"} {
		file := filepath.Join(folder, tool+".json")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var input hackbot.ScannerInput
		if json.Unmarshal(data, &input) != nil || input.Tool != tool {
			t.Fatal("wrong scanner artifact")
		}
		stat, _ := os.Stat(file)
		if stat.Mode().Perm() != 0600 {
			t.Fatal("scanner input is not private")
		}
	}
	if _, err = writeScannerInputs(root, inputs); err != nil {
		t.Fatal("identical handoff is not idempotent", err)
	}
	otherRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(otherRoot, "scanner-inputs")); err != nil {
		t.Fatal(err)
	}
	if _, err = writeScannerInputs(otherRoot, inputs); err == nil {
		t.Fatal("symlink handoff directory accepted")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("handoff escaped private engagement directory")
	}
	inputs[0].Target = "different.test"
	if _, err = writeScannerInputs(root, inputs); err == nil {
		t.Fatal("changed handoff overwritten")
	}
}

func TestClaimCanCarryFullScannerFindingSnapshots(t *testing.T) {
	body := `{"observations":"` + strings.Repeat("x", 2<<20) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "fixture")
	var out map[string]string
	if err := client.post(context.Background(), "executions/fixture/claim", nil, &out); err != nil || len(out["observations"]) != 2<<20 {
		t.Fatalf("claim truncated scanner findings: %v", err)
	}
	if err := client.post(context.Background(), "heartbeat", nil, &out); err == nil {
		t.Fatal("ordinary response limit was widened")
	}
}
