package launcher_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pentestharnes "github.com/r1pu5u/pentest-harnes"
	"github.com/r1pu5u/pentest-harnes/internal/launcher"
)

func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, ".pi", "agent"))
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("PENTEST_HARNES_SUBAGENTS_DIR", "")
	t.Setenv("BILI_CONFIG_FILE", "")
	t.Setenv("PENTEST_HARNES_STACK", "")
	t.Setenv("PENTEST_HARNES_JOB_STATUS_FILE", "")
	return home
}

func TestServiceJobMetadataIsPrivateAndMatchesSelectedSession(t *testing.T) {
	isolate(t)
	status := filepath.Join(t.TempDir(), "job.json")
	t.Setenv("PENTEST_HARNES_JOB_STATUS_FILE", status)
	executable, _ := os.Executable()
	var output bytes.Buffer
	err := launcher.Run(launcher.Options{Dir: t.TempDir(), Pi: executable, New: true, RPC: true, Execute: func(command *exec.Cmd) error {
		data, err := os.ReadFile(status)
		if err != nil {
			t.Fatal(err)
		}
		var metadata struct {
			SessionID string `json:"sessionId"`
			Root      string `json:"root"`
			Workspace string `json:"workspace"`
		}
		if err := json.Unmarshal(data, &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.Workspace != command.Dir || metadata.Root != filepath.Dir(command.Dir) || metadata.SessionID != filepath.Base(metadata.Root) {
			t.Fatalf("wrong job metadata: %s", data)
		}
		info, _ := os.Stat(status)
		if os.PathSeparator != '\\' && info.Mode().Perm() != 0600 {
			t.Fatal("job metadata is not private")
		}
		return nil
	}}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDockerGoriConfigurationAndOfflineOptOut(t *testing.T) {
	isolate(t)
	t.Setenv("PENTEST_HARNES_STACK", "/opt/pentest/docker/stack.py")
	executable, _ := os.Executable()
	for _, offline := range []bool{false, true} {
		var output bytes.Buffer
		err := launcher.Run(launcher.Options{Dir: t.TempDir(), Pi: executable, New: true, Offline: offline, Execute: func(command *exec.Cmd) error {
			data, err := os.ReadFile(filepath.Join(command.Dir, ".pi", "mcp.json"))
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Servers map[string]json.RawMessage `json:"mcpServers"`
			}
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatal(err)
			}
			_, hasGori := config.Servers["gori"]
			if hasGori == offline {
				t.Fatalf("Gori configuration offline=%v: %s", offline, data)
			}
			env := strings.Join(command.Env, "\n")
			if !offline && (!strings.Contains(env, "PENTEST_HARNES_GORI_DB=") || !strings.Contains(env, "INTERCEPTOR_TEMP=")) {
				t.Fatal("missing engagement environment")
			}
			return nil
		}}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSessionIsolationContinuationAndFreshLaunchBudgets(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var launches []string
	var workspaces []string
	var sessionIDs []string
	execute := func(command *exec.Cmd) error {
		workspaces = append(workspaces, command.Dir)
		joined := strings.Join(command.Args, " ")
		if !strings.Contains(joined, "--session-id") || !strings.Contains(joined, "extension.ts") || !strings.Contains(joined, "ORCHESTRATOR.md") || strings.Contains(joined, "--no-tools") {
			t.Fatalf("incorrect Pi contract: %v", command.Args)
		}
		if !strings.Contains(joined, "builtin:mcp") || strings.Contains(joined, "--mcp-config") {
			t.Fatalf("launcher did not use native MCP: %v", command.Args)
		}
		for _, env := range command.Env {
			if strings.HasPrefix(env, "PENTEST_HARNES_LAUNCH=") {
				file := strings.TrimPrefix(env, "PENTEST_HARNES_LAUNCH=")
				launches = append(launches, file)
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var budget struct {
					BudgetUSD, SpentUSD float64
					BudgetMode          string
				}
				if err := json.Unmarshal(data, &budget); err != nil || budget.BudgetUSD != 15 || budget.SpentUSD != 0 || budget.BudgetMode != "estimate" {
					t.Fatalf("incorrect launch budget: %s", data)
				}
			}
		}
		for _, relative := range []string{"AGENTS.md", ".pi/mcp.json", ".pi/skills/pentest/SKILL.md", ".pi/skills/pentest/references/checklist.md"} {
			if _, err := os.Stat(filepath.Join(command.Dir, relative)); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}
	var output bytes.Buffer
	run := func(options launcher.Options, input string) {
		t.Helper()
		options.Dir, options.Pi, options.Execute = root, executable, execute
		if err := launcher.Run(options, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(input), &output, &output); err != nil {
			t.Fatal(err)
		}
		sessions, err := launcher.Sessions(root)
		if err != nil {
			t.Fatal(err)
		}
		sessionIDs = append(sessionIDs, sessions[0].ID)
	}
	run(launcher.Options{New: true, Name: "First engagement"}, "")
	legacyMCP := filepath.Join(filepath.Dir(workspaces[0]), "private", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(legacyMCP), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyMCP, []byte(`{"mcpServers":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	run(launcher.Options{Session: sessionIDs[0]}, "")
	if _, err := os.Stat(legacyMCP); !os.IsNotExist(err) {
		t.Fatal("legacy managed MCP configuration was not migrated")
	}
	run(launcher.Options{New: true, Name: "Independent engagement"}, "")
	if workspaces[0] != workspaces[1] || workspaces[0] == workspaces[2] {
		t.Fatalf("session workspaces not isolated/resumable: %v", workspaces)
	}
	if launches[0] == launches[1] || launches[1] == launches[2] {
		t.Fatal("launches reused a budget ledger")
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("launcher overwrote project instructions")
	}
	sessions, _ := launcher.Sessions(root)
	selected, isNew, err := launcher.Select(root, launcher.Options{}, strings.NewReader("1\n"), &output)
	if err != nil || isNew || selected.ID != sessions[0].ID {
		t.Fatalf("cannot interactively resume: %v", err)
	}
	if _, _, err := launcher.Select(root, launcher.Options{}, strings.NewReader(""), &output); err == nil {
		t.Fatal("EOF silently selected/created a session")
	}
}

func TestDryRunAndMissingPiDoNotWriteSessionFiles(t *testing.T) {
	isolate(t)
	root := filepath.Join(t.TempDir(), "new project")
	executable, _ := os.Executable()
	var output bytes.Buffer
	if err := launcher.Run(launcher.Options{Dir: root, Pi: executable, DryRun: true, Execute: func(*exec.Cmd) error { t.Fatal("dry run executed Pi"); return nil }}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("dry run wrote files")
	}
	if err := launcher.Run(launcher.Options{Dir: root, Pi: filepath.Join(t.TempDir(), "missing")}, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output); err == nil {
		t.Fatal("missing Pi executable accepted")
	}
}

func TestConcurrentSessionLaunchIsRejected(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	executable, _ := os.Executable()
	var output bytes.Buffer
	options := launcher.Options{Dir: root, Pi: executable, New: true}
	options.Execute = func(command *exec.Cmd) error {
		sessions, err := launcher.Sessions(root)
		if err != nil {
			t.Fatal(err)
		}
		other := launcher.Options{Dir: root, Pi: executable, Session: sessions[0].ID, Execute: func(*exec.Cmd) error { t.Fatal("concurrent session executed Pi"); return nil }}
		if err := launcher.Run(other, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output); err == nil || !strings.Contains(err.Error(), "locked") {
			t.Fatalf("concurrent launch not rejected: %v", err)
		}
		return nil
	}
	if err := launcher.Run(options, pentestharnes.Skills(), pentestharnes.Runtime(), strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
}
