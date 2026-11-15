package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pentestharnes "github.com/r1pu5u/pentest-harnes"
	"github.com/r1pu5u/pentest-harnes/internal/cli"
)

func TestPreviewAndInstallFromUnrelatedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project with spaces")
	var stdout, stderr bytes.Buffer
	args := []string{"init", "--dir", root, "--agent", "codex", "--dry-run"}
	if code := cli.Run(args, pentestharnes.Skills(), "test", &stdout, &stderr); code != 0 {
		t.Fatalf("preview failed: %s", stderr.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("dry run created destination")
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run(args[:len(args)-1], pentestharnes.Skills(), "test", &stdout, &stderr); code != 0 {
		t.Fatalf("install failed: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/pentest/references/gori.md")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pentest-gadgets", "wordpress-pentest"} {
		if _, err := os.Stat(filepath.Join(root, ".agents/skills", name)); !os.IsNotExist(err) {
			t.Fatalf("removed skill %s was installed: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatal("unexpected target")
	}
}

func TestCommandContracts(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
		text string
	}{
		{"help", []string{"--help"}, 0, "pentest-harnes init"},
		{"version", []string{"version"}, 0, "v1.2.3"},
		{"list", []string{"list"}, 0, "pentest-reflection"},
		{"unknown", []string{"missing"}, 2, "unknown command"},
		{"bad flag", []string{"init", "--missing"}, 2, "flag provided but not defined"},
		{"default bad flag", []string{"--missing"}, 2, "flag provided but not defined"},
		{"positional", []string{"init", "unexpected"}, 2, "unexpected arguments"},
		{"bad agent", []string{"init", "--agent", "unknown"}, 1, "unknown agent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run(test.args, pentestharnes.Skills(), "v1.2.3", &stdout, &stderr)
			if code != test.code || !strings.Contains(stdout.String()+stderr.String(), test.text) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestInitInstallsOrchestratorAndAllSkills(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"init"}, pentestharnes.Skills(), "test", &stdout, &stderr); code != 0 {
		t.Fatalf("init failed: %s", stderr.String())
	}
	for _, path := range []string{
		"AGENTS.md", "CLAUDE.md",
		".agent/skills/pentest/SKILL.md",
		".agents/skills/pentest/SKILL.md",
		".claude/skills/pentest/SKILL.md",
		".pi/skills/pentest/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("init did not install %s: %v", path, err)
		}
	}
	if !strings.Contains(stdout.String(), "Installed orchestrator and 19 skills") {
		t.Fatalf("unexpected installation summary: %s", stdout.String())
	}
}

func TestInitFlagsPreviewInstructionsWithoutWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	var stdout, stderr bytes.Buffer
	args := []string{"init", "--dir", root, "--agent", "claude", "--dry-run"}
	if code := cli.Run(args, pentestharnes.Skills(), "test", &stdout, &stderr); code != 0 {
		t.Fatalf("preview failed: %s", stderr.String())
	}
	for _, text := range []string{"create AGENTS.md", "create CLAUDE.md", ".claude/skills/pentest/SKILL.md"} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("preview missing %q", text)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("default preview created destination")
	}
}
