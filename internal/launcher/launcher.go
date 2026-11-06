// Package launcher runs Pi in isolated, persistent engagement workspaces.
package launcher

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/r1pu5u/pentest-harnes/internal/installer"
)

type Options struct {
	Dir, Session, Name, Pi, OpenCodeConfig, Prompt string
	New, DryRun, Print, Offline, RPC               bool
	// Execute is optional and allows tests to inspect the actual process contract.
	Execute func(*exec.Cmd) error
}

type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	Root      string `json:"-"`
}

func Sessions(root string) ([]Session, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".pentest-harnes", "sessions"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sessions []Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, ".pentest-harnes", "sessions", entry.Name())
		data, err := os.ReadFile(filepath.Join(path, "session.json"))
		if err != nil {
			return nil, fmt.Errorf("read session %s: %w", entry.Name(), err)
		}
		var session Session
		if err := json.Unmarshal(data, &session); err != nil || session.ID != entry.Name() || len(session.ID) < 8 {
			return nil, fmt.Errorf("invalid session metadata: %s", entry.Name())
		}
		session.Root = path
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].CreatedAt > sessions[j].CreatedAt })
	return sessions, nil
}

func uuid() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	s := hex.EncodeToString(value[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}

func privateJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".harness-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func Select(root string, options Options, stdin io.Reader, stdout io.Writer) (Session, bool, error) {
	sessions, err := Sessions(root)
	if err != nil {
		return Session{}, false, err
	}
	if options.New && options.Session != "" {
		return Session{}, false, errors.New("--new and --session cannot be combined")
	}
	if options.Session != "" {
		var matched []Session
		for _, session := range sessions {
			if session.ID == options.Session || strings.HasPrefix(session.ID, options.Session) {
				matched = append(matched, session)
			}
		}
		if len(matched) != 1 {
			return Session{}, false, errors.New("session ID must match exactly one existing session")
		}
		return matched[0], false, nil
	}
	if !options.New && len(sessions) > 0 {
		fmt.Fprintln(stdout, "Pentest sessions:")
		fmt.Fprintln(stdout, "  0  Start a new session")
		for i, session := range sessions {
			fmt.Fprintf(stdout, "  %d  %s  %s  %s\n", i+1, session.ID[:8], session.Name, session.CreatedAt)
		}
		fmt.Fprint(stdout, "Continue a session or start new [number]: ")
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && len(line) == 0 {
			return Session{}, false, errors.New("select a session interactively or use --session ID / --new")
		}
		choice, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || choice < 0 || choice > len(sessions) {
			return Session{}, false, errors.New("invalid session selection")
		}
		if choice > 0 {
			return sessions[choice-1], false, nil
		}
	}
	id, err := uuid()
	if err != nil {
		return Session{}, false, err
	}
	name := options.Name
	if name == "" {
		name = "Engagement " + time.Now().Format("2006-01-02 15:04")
	}
	return Session{ID: id, Name: name, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Root: filepath.Join(root, ".pentest-harnes", "sessions", id)}, true, nil
}

func agentDirectory(home string) string {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		if absolute, err := filepath.Abs(dir); err == nil {
			return absolute
		}
		return dir
	}
	return filepath.Join(home, ".pi", "agent")
}

func externalSkills(home string) []string {
	var paths []string
	for _, name := range []string{"burpsuite-mcp", "caido-mode"} {
		for _, dir := range []string{filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".config", "opencode", "skills"), filepath.Join(home, ".agents", "skills")} {
			path := filepath.Join(dir, name, "SKILL.md")
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
				break
			}
		}
	}
	return paths
}

// Run starts interactive Pi using the user's existing provider configuration.
func Run(options Options, source, runtime fs.FS, stdin io.Reader, stdout, stderr io.Writer) error {
	if options.RPC && !options.New && options.Session == "" {
		return errors.New("RPC mode requires --new or --session ID")
	}
	root, err := filepath.Abs(options.Dir)
	if err != nil {
		return err
	}
	pi := options.Pi
	if pi == "" {
		pi = "pi"
	}
	executable, err := exec.LookPath(pi)
	if err != nil {
		return errors.New("Pi is not installed; install @earendil-works/pi-coding-agent, then run pi to configure your provider")
	}
	if options.Execute == nil {
		version, err := exec.Command(executable, "--version").Output()
		var major, minor, patch int
		if err != nil {
			return fmt.Errorf("cannot determine Pi version: %w", err)
		}
		if n, _ := fmt.Sscanf(strings.TrimSpace(string(version)), "%d.%d.%d", &major, &minor, &patch); n != 3 || major < 1 || (major == 1 && minor < 5) {
			return errors.New("Pi 1.5.0 or newer is required for built-in MCP and request-budget hooks; update Pi before launching")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agentDir := agentDirectory(home)
	packages := installedPackages(agentDir)
	mcp, imported, err := MCPConfig(options.OpenCodeConfig, home, agentDir)
	if err != nil {
		return err
	}
	if options.DryRun {
		names, err := installer.SkillNames(source)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Would launch %s in an isolated session under %s\n", executable, filepath.Join(root, ".pentest-harnes", "sessions"))
		fmt.Fprintf(stdout, "Bundle: orchestrator, %d skills, workflow controller; estimated spend ($15 reporting reference, no hard monetary cap); managed traffic ceiling 50 requests/second.\nMCP imports: %s\nNo files written.\n", len(names), strings.Join(imported, ", "))
		return nil
	}
	diagnostics := stdout
	if options.RPC {
		diagnostics = stderr
	}
	session, isNew, err := Select(root, options, stdin, diagnostics)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(session.Root, 0700); err != nil {
		return err
	}
	ignore, err := os.OpenFile(filepath.Join(root, ".pentest-harnes", ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := ignore.WriteString("*\n")
		if err := errors.Join(writeErr, ignore.Close()); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrExist) {
		return err
	}
	// One launcher owns an engagement at a time. Never automatically steal a
	// lock: an interrupted mutation must be reconciled before resuming.
	lockPath := filepath.Join(session.Root, "launch.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("session is locked; confirm its launcher and Pi processes have exited, inspect interrupted work, then remove %s: %w", lockPath, err)
	}
	fmt.Fprintf(lock, "%d\n", os.Getpid())
	lock.Close()
	defer os.Remove(lockPath)
	if isNew {
		if err := privateJSON(filepath.Join(session.Root, "session.json"), session); err != nil {
			return err
		}
	}
	workspace := filepath.Join(session.Root, "workspace")
	plan, err := installer.NewPlan(source, workspace, "pi", true)
	if err != nil {
		return err
	}
	if err := plan.Apply(); err != nil {
		return err
	}
	// Each model attempt must pass the extension's reporting hook. Disable opaque
	// provider retries and let errors become checkpointed exceptions.
	if err := privateJSON(filepath.Join(workspace, ".pi", "settings.json"), map[string]any{"tuiMode": "fullscreen", "retry": map[string]any{"enabled": false, "provider": map[string]any{"maxRetries": 0}}}); err != nil {
		return err
	}
	if err := writeLeafProfile(workspace, runtime); err != nil {
		return err
	}
	var biliPath string
	if packages.BillionContext != "" {
		biliPath, err = billionConfig(session.Root, home)
		if err != nil {
			return err
		}
	}
	runtimeDir := filepath.Join(session.Root, "runtime")
	if err := fs.WalkDir(runtime, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := fs.ReadFile(runtime, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(runtimeDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0600)
	}); err != nil {
		return err
	}
	launchID, err := uuid()
	if err != nil {
		return err
	}
	launchPath := filepath.Join(session.Root, "launches", launchID+".json")
	if err := privateJSON(launchPath, map[string]any{"id": launchID, "budgetMode": "estimate", "budgetUSD": 15, "spentUSD": 0, "reservations": map[string]any{}, "events": map[string]any{}, "startedAt": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		return err
	}
	mcpPath := filepath.Join(workspace, ".pi", "mcp.json")
	if err := privateJSON(mcpPath, mcp); err != nil {
		return err
	}
	// This was the managed config location used by older harness launches.
	if err := os.Remove(filepath.Join(session.Root, "private", "mcp.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	piSessions := filepath.Join(session.Root, "pi")
	if err := os.MkdirAll(piSessions, 0700); err != nil {
		return err
	}
	args := []string{"--approve", "--session-dir", piSessions, "--session-id", session.ID, "--extension", "builtin:mcp", "--extension", "builtin:codemode", "--extension", "builtin:tool-search", "--extension", filepath.Join(runtimeDir, "extension.ts"), "--pentest-runtime", "--append-system-prompt", filepath.Join(runtimeDir, "ORCHESTRATOR.md")}
	if packages.Subagents != "" {
		// Pi deduplicates resources also discovered through installed packages.
		args = append(args, "--extension", filepath.Join(packages.Subagents, "bundle", "index.js"))
	}
	for _, skill := range externalSkills(home) {
		args = append(args, "--skill", skill)
	}
	if options.Offline {
		args = append(args, "--offline")
	}
	if options.Print {
		args = append(args, "--print")
	}
	if options.RPC {
		args = append(args, "--mode", "rpc")
	}
	if options.Prompt != "" {
		args = append(args, options.Prompt)
	}
	command := exec.Command(executable, args...)
	command.Dir = workspace
	command.Env = append(os.Environ(),
		"PI_CODING_AGENT_DIR="+agentDir,
		"PENTEST_HARNES_SESSION="+session.Root,
		"PENTEST_HARNES_LAUNCH="+launchPath,
		"PENTEST_HARNES_PI="+executable,
		"PENTEST_HARNES_TASK=",
		"PENTEST_HARNES_EXTERNAL_SKILLS="+strings.Join(externalSkills(home), string(os.PathListSeparator)),
		"PENTEST_HARNES_SUBAGENTS_DIR="+packages.Subagents,
	)
	if biliPath != "" {
		command.Env = append(command.Env, "BILI_CONFIG_FILE="+biliPath)
	}
	// Collapse inherited overrides so inspection and execution see the same
	// canonical package root and session-private config path.
	command.Env = command.Environ()
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	fmt.Fprintf(diagnostics, "Session: %s (%s)\nWorkspace: %s\nEstimated spend: $15 reporting reference, no hard monetary cap; managed traffic ceiling: 50 requests/second\n", session.Name, session.ID, workspace)
	if options.Execute != nil {
		return options.Execute(command)
	}
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case received := <-interrupts:
				_ = command.Process.Signal(received)
			case <-done:
				return
			}
		}
	}()
	if err := command.Wait(); err != nil {
		return fmt.Errorf("Pi exited: %w (session saved; relaunch to continue)", err)
	}
	return nil
}
