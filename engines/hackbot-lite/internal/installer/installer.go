// Package installer plans and installs project-local orchestrator instructions and skills.
package installer

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var targetDirs = map[string]string{
	"agent":  ".agent/skills",
	"codex":  ".agents/skills",
	"claude": ".claude/skills",
	"pi":     ".pi/skills",
}

// Entry describes one bundled file and the change it needs.
type Entry struct {
	Path   string
	Action string
	data   []byte
	before []byte
}

// Plan is fully checked for conflicts before any directories or files are made.
type Plan struct {
	Root         string
	Skills       []string
	Targets      []string
	Instructions []string
	Entries      []Entry
}

// SkillNames lists the directories under skills/ in the bundle.
func SkillNames(source fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(source, "skills")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := fs.ReadFile(source, "skills/"+entry.Name()+"/SKILL.md"); err != nil {
			return nil, fmt.Errorf("invalid bundled skill %s: %w", entry.Name(), err)
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, errors.New("no bundled skills found")
	}
	sort.Strings(names)
	return names, nil
}

// NewPlan accepts all, agent, codex, claude, or pi as the destination selector.
// An existing file with different content is a conflict unless force is true.
func NewPlan(source fs.FS, root, agent string, force bool) (*Plan, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var targets []string
	if agent == "all" {
		targets = []string{"agent", "codex", "claude", "pi"}
	} else if _, ok := targetDirs[agent]; ok {
		targets = []string{agent}
	} else {
		return nil, fmt.Errorf("unknown agent %q: choose all, agent, codex, claude, or pi", agent)
	}
	names, err := SkillNames(source)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Root: absolute, Skills: names}
	var conflicts []string
	addFile := func(relative string, data []byte) error {
		if err := checkPath(absolute, relative); err != nil {
			return err
		}
		entry := Entry{Path: relative, Action: "create", data: data}
		existing, err := os.ReadFile(filepath.Join(absolute, relative))
		switch {
		case err == nil:
			entry.before = existing
			if bytes.Equal(existing, data) {
				entry.Action = "unchanged"
			} else if force {
				entry.Action = "replace"
			} else {
				conflicts = append(conflicts, relative)
			}
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return fmt.Errorf("inspect %s: %w", relative, err)
		}
		plan.Entries = append(plan.Entries, entry)
		return nil
	}
	orchestrator, err := fs.ReadFile(source, "AGENTS.md")
	if err != nil {
		return nil, fmt.Errorf("read bundled orchestrator: %w", err)
	}
	// Keep the source document useful in this checkout while routing installed
	// links to a selected skill folder. All-agent installs use the compatibility folder.
	instructions := string(orchestrator)
	for _, name := range names {
		instructions = strings.ReplaceAll(instructions, "](skills/"+name+"/", "]("+targetDirs[targets[0]]+"/"+name+"/")
	}
	if err := addFile("AGENTS.md", []byte(instructions)); err != nil {
		return nil, err
	}
	plan.Instructions = append(plan.Instructions, "AGENTS.md")
	if agent == "all" || agent == "claude" {
		if err := addFile("CLAUDE.md", []byte("@AGENTS.md\n")); err != nil {
			return nil, err
		}
		plan.Instructions = append(plan.Instructions, "CLAUDE.md")
	}
	for _, target := range targets {
		targetDir := targetDirs[target]
		plan.Targets = append(plan.Targets, targetDir)
		for _, name := range names {
			err = fs.WalkDir(source, "skills/"+name, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if !d.Type().IsRegular() {
					return fmt.Errorf("unsupported bundled file: %s", path)
				}
				relative := filepath.Join(filepath.FromSlash(targetDir), filepath.FromSlash(strings.TrimPrefix(path, "skills/")))
				data, err := fs.ReadFile(source, path)
				if err != nil {
					return err
				}
				return addFile(relative, data)
			})
			if err != nil {
				return nil, err
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%d existing file(s) differ, including %s; no files changed (use --force to replace bundled files)", len(conflicts), conflicts[0])
	}
	return plan, nil
}

// checkPath rejects symlinks and incompatible nodes inside the destination.
func checkPath(root, relative string) error {
	if !filepath.IsLocal(relative) {
		return fmt.Errorf("destination must stay inside the project: %s", relative)
	}
	path := root
	parts := []string{}
	for dir := relative; dir != "."; dir = filepath.Dir(dir) {
		parts = append(parts, filepath.Base(dir))
	}
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("project directory is not a regular directory: %s", root)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for i := len(parts) - 1; i >= 0; i-- {
		path = filepath.Join(path, parts[i])
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink destination: %s", path)
		}
		if (i > 0 && !info.IsDir()) || (i == 0 && !info.Mode().IsRegular()) {
			return fmt.Errorf("incompatible destination: %s", path)
		}
	}
	return nil
}

// Apply writes only bundled files; unrelated local files are preserved.
// Preflight protects against known conflicts, but an I/O failure can leave a
// partial installation. It is safe to rerun after resolving that failure.
func (plan *Plan) Apply() error {
	for _, entry := range plan.Entries {
		if err := checkPath(plan.Root, entry.Path); err != nil {
			return err
		}
		path := filepath.Join(plan.Root, entry.Path)
		current, err := os.ReadFile(path)
		if entry.Action == "create" {
			if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("destination changed after planning: %s", entry.Path)
			}
		} else if err != nil || !bytes.Equal(current, entry.before) {
			return fmt.Errorf("destination changed after planning: %s", entry.Path)
		}
	}
	for _, entry := range plan.Entries {
		if entry.Action == "unchanged" {
			continue
		}
		if err := checkPath(plan.Root, entry.Path); err != nil {
			return err
		}
		path := filepath.Join(plan.Root, entry.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if entry.Action == "create" {
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			_, writeErr := file.Write(entry.data)
			closeErr := file.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				_ = os.Remove(path)
				return err
			}
		} else {
			if err := replaceFile(path, entry.data); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".pentest-harnes-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	chmodErr := file.Chmod(0644)
	closeErr := file.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
