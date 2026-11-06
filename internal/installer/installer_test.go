package installer_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	pentestharnes "github.com/r1pu5u/pentest-harnes"
	"github.com/r1pu5u/pentest-harnes/internal/installer"
)

func smallBundle() fs.FS {
	return fstest.MapFS{
		"AGENTS.md":                              {Data: []byte("# Orchestrator\nUse [pentest](skills/pentest/SKILL.md).\n")},
		"skills/pentest/SKILL.md":                {Data: []byte("main skill")},
		"skills/pentest/references/terms.md":     {Data: []byte("shared terms")},
		"skills/pentest-gadgets/SKILL.md":        {Data: []byte("gadget skill")},
		"skills/pentest-gadgets/references/a.md": {Data: []byte("[terms](../../pentest/references/terms.md)")},
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBundledInstallPreservesEveryFileAndReference(t *testing.T) {
	root := t.TempDir()
	source := pentestharnes.Skills()
	plan, err := installer.NewPlan(source, root, "all", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Skills) != 21 || len(plan.Targets) != 4 {
		t.Fatalf("unexpected bundle: %d skills, %d targets", len(plan.Skills), len(plan.Targets))
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	for _, target := range plan.Targets {
		for _, skill := range plan.Skills {
			err := fs.WalkDir(source, "skills/"+skill, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				want, err := fs.ReadFile(source, path)
				if err != nil {
					return err
				}
				got, err := os.ReadFile(filepath.Join(root, target, filepath.FromSlash(strings.TrimPrefix(path, "skills/"))))
				if err != nil {
					return err
				}
				if !bytes.Equal(got, want) {
					t.Errorf("changed bytes: %s/%s", target, path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	again, err := installer.NewPlan(source, root, "all", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range again.Entries {
		if entry.Action != "unchanged" {
			t.Errorf("rerun would change %s", entry.Path)
		}
	}
	if err := again.Apply(); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledOrchestratorReferencesResolveForEachAgent(t *testing.T) {
	links := regexp.MustCompile(`\[[^\]]+\]\(([^)\s]+)\)`)
	for _, agent := range []string{"all", "agent", "codex", "claude", "pi"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			plan, err := installer.NewPlan(pentestharnes.Skills(), root, agent, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(); err != nil {
				t.Fatal(err)
			}
			instructions := read(t, filepath.Join(root, "AGENTS.md"))
			matches := links.FindAllStringSubmatch(instructions, -1)
			if len(matches) < 21 {
				t.Fatalf("missing specialist routing: %s", instructions)
			}
			for _, match := range matches {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(match[1]))); err != nil {
					t.Errorf("unresolved installed reference %q: %v", match[1], err)
				}
			}
			claude := filepath.Join(root, "CLAUDE.md")
			if agent == "all" || agent == "claude" {
				if got := read(t, claude); got != "@AGENTS.md\n" {
					t.Fatalf("Claude Code does not load orchestrator: %q", got)
				}
			} else if _, err := os.Stat(claude); !os.IsNotExist(err) {
				t.Fatal("unexpected Claude Code instructions")
			}
		})
	}
}

func TestInstructionConflictsRequireForceBeforeAnyWrites(t *testing.T) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, name)
			write(t, path, "existing project instructions")
			if _, err := installer.NewPlan(smallBundle(), root, "all", false); err == nil {
				t.Fatal("expected instruction conflict")
			}
			if got := read(t, path); got != "existing project instructions" {
				t.Fatal("existing instructions changed")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 {
				t.Fatalf("conflict wrote files: entries=%v err=%v", entries, err)
			}
			plan, err := installer.NewPlan(smallBundle(), root, "all", true)
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Apply(); err != nil {
				t.Fatal(err)
			}
			if got := read(t, path); got == "existing project instructions" {
				t.Fatal("force did not replace instructions")
			}
		})
	}
}

func TestConflictIsDetectedBeforeAnyOtherWrites(t *testing.T) {
	root := t.TempDir()
	conflict := filepath.Join(root, ".claude/skills/pentest/SKILL.md")
	write(t, conflict, "local customization")
	if _, err := installer.NewPlan(smallBundle(), root, "all", false); err == nil {
		t.Fatal("expected conflict")
	}
	if _, err := os.Stat(filepath.Join(root, ".agent")); !os.IsNotExist(err) {
		t.Fatal("another destination was created before conflict resolution")
	}
	if got := read(t, conflict); got != "local customization" {
		t.Fatal("existing file changed")
	}
}

func TestForceOnlyReplacesBundledFiles(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, ".claude/skills/pentest/SKILL.md")
	custom := filepath.Join(root, ".claude/skills/pentest/custom.md")
	settings := filepath.Join(root, ".claude/settings.json")
	other := filepath.Join(root, ".claude/skills/local-skill/SKILL.md")
	for _, path := range []string{managed, custom, settings, other} {
		write(t, path, "keep local")
	}
	plan, err := installer.NewPlan(smallBundle(), root, "claude", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, managed); got != "main skill" {
		t.Fatal("bundled skill was not replaced")
	}
	for _, path := range []string{custom, settings, other} {
		if got := read(t, path); got != "keep local" {
			t.Errorf("unrelated file changed: %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".agents")); !os.IsNotExist(err) {
		t.Fatal("single-agent selection installed another destination")
	}
}

func TestSymlinkDestinationsRejectedEvenWithForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	for _, relative := range []string{"AGENTS.md", "CLAUDE.md", ".claude", ".claude/skills/pentest/SKILL.md"} {
		t.Run(relative, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(root, relative)
			if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
				t.Fatal(err)
			}
			target := outside
			if strings.HasSuffix(relative, ".md") {
				target = filepath.Join(outside, "protected.md")
				write(t, target, "protected")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if _, err := installer.NewPlan(smallBundle(), root, "all", true); err == nil {
				t.Fatal("expected symlink rejection")
			}
			if _, err := os.Stat(filepath.Join(root, ".agent")); !os.IsNotExist(err) {
				t.Fatal("other destination changed")
			}
		})
	}
}

func TestPlanningDoesNotCreateMissingProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new project")
	if _, err := installer.NewPlan(smallBundle(), root, "all", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("planning wrote to disk")
	}
}

func TestApplyRejectsFilesChangedAfterPlanningBeforeWriting(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, ".claude/skills/pentest/SKILL.md")
	write(t, managed, "before")
	plan, err := installer.NewPlan(smallBundle(), root, "all", true)
	if err != nil {
		t.Fatal(err)
	}
	write(t, managed, "changed by another process")
	if err := plan.Apply(); err == nil {
		t.Fatal("expected stale-plan rejection")
	}
	if got := read(t, managed); got != "changed by another process" {
		t.Fatal("concurrent edit overwritten")
	}
	if _, err := os.Stat(filepath.Join(root, ".agent")); !os.IsNotExist(err) {
		t.Fatal("wrote files before stale-plan check")
	}
}

func TestInvalidTargetsAndDestinationNodes(t *testing.T) {
	root := t.TempDir()
	if _, err := installer.NewPlan(smallBundle(), root, "../../outside", false); err == nil {
		t.Fatal("invalid target accepted")
	}
	write(t, filepath.Join(root, ".claude"), "file in place of folder")
	if _, err := installer.NewPlan(smallBundle(), root, "claude", true); err == nil {
		t.Fatal("incompatible ancestor accepted")
	}
	if _, err := installer.NewPlan(fstest.MapFS{}, root, "all", false); err == nil {
		t.Fatal("empty bundle accepted")
	}
}
