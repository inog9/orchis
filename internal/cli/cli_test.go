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
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/pentest-gadgets/references/research/framework-cspt.md")); err != nil {
		t.Fatal(err)
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
		{"list", []string{"list"}, 0, "pentest-gadgets"},
		{"unknown", []string{"missing"}, 2, "unknown command"},
		{"bad flag", []string{"init", "--missing"}, 2, "flag provided but not defined"},
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
