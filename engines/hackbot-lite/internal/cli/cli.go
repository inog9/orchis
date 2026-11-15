package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	pentestharnes "github.com/r1pu5u/pentest-harnes"
	"github.com/r1pu5u/pentest-harnes/internal/installer"
	"github.com/r1pu5u/pentest-harnes/internal/launcher"
)

const help = `pentest-harnes — persistent Pi pentest orchestrator

Usage:
  pentest-harnes [--dir PATH] [--new | --session ID] [--name NAME]
                [--pi PATH] [--opencode-config PATH] [--offline] [--dry-run]
                [--print --prompt TEXT | --rpc]
  pentest-harnes sessions [--dir PATH]
  pentest-harnes doctor [--opencode-config PATH]
  pentest-harnes init [--dir PATH] [--agent all|agent|codex|claude|pi] [--dry-run] [--force]
  pentest-harnes list
  pentest-harnes version

Running without a subcommand opens Pi. Choose a saved session or start a new one.
Each session has an isolated workspace, orchestrator, all skills, and checkpoints
under .pentest-harnes/sessions. Existing Pi provider credentials are reused.
The orchestrator chooses goal-driven specialist assignments from your request.
The lead has full tools, including bash, and can delegate parallel specialist batches.
Inside Pi, /pentest-agents shows actual worker PIDs, progress, and queued work.
With pi-subagents installed, /subagents-fleet opens selectable live transcripts.
Spend estimates reset per launch (no hard monetary cap); managed traffic <=50 req/s.
There is no fixed runtime: reflection/hypothesis loops drive checklist completion.
Specialists retain full tools. Shell/browser traffic outside the managed
scheduler cannot be rate-enforced by the harness.

init only installs files, without starting Pi; conflicts require --force.
The binary bundles the orchestrator, controller, skills, and references.
`

// Run executes the CLI without exiting, allowing isolated end-to-end tests.
func Run(args []string, source fs.FS, version string, stdout, stderr io.Writer) int {
	return RunWithInput(args, source, version, os.Stdin, stdout, stderr)
}

// RunWithInput allows session selection without relying on the process stdin.
func RunWithInput(args []string, source fs.FS, version string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || (strings.HasPrefix(args[0], "-") && args[0] != "--help" && args[0] != "-h" && args[0] != "--version") {
		args = append([]string{"run"}, args...)
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, help)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "pentest-harnes %s\n", version)
		return 0
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		flags.SetOutput(stderr)
		options := launcher.Options{}
		flags.StringVar(&options.Dir, "dir", ".", "project directory containing sessions")
		flags.StringVar(&options.Session, "session", "", "continue an existing session ID or unique prefix")
		flags.StringVar(&options.Name, "name", "", "new session display name")
		flags.StringVar(&options.Pi, "pi", "pi", "Pi executable")
		flags.StringVar(&options.OpenCodeConfig, "opencode-config", "", "OpenCode MCP configuration path")
		flags.StringVar(&options.Prompt, "prompt", "", "optional initial prompt")
		flags.BoolVar(&options.New, "new", false, "start a new session")
		flags.BoolVar(&options.DryRun, "dry-run", false, "preview launch without writing files")
		flags.BoolVar(&options.Print, "print", false, "run Pi in non-interactive print mode")
		flags.BoolVar(&options.Offline, "offline", false, "disable Pi startup network operations")
		flags.BoolVar(&options.RPC, "rpc", false, "Pi RPC mode for programmatic clients")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if flags.NArg() != 0 || (options.Print && options.Prompt == "") || (options.Print && options.RPC) {
			fmt.Fprintln(stderr, "use --prompt TEXT for a prompt; --print requires --prompt")
			return 2
		}
		if err := launcher.Run(options, source, pentestharnes.Runtime(), stdin, stdout, stderr); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "sessions":
		flags := flag.NewFlagSet("sessions", flag.ContinueOnError)
		flags.SetOutput(stderr)
		dir := flags.String("dir", ".", "project directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return 2
		}
		sessions, err := launcher.Sessions(*dir)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, session := range sessions {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", session.ID, session.CreatedAt, session.Name)
		}
		return 0
	case "doctor":
		flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
		flags.SetOutput(stderr)
		config := flags.String("opencode-config", "", "OpenCode MCP configuration path")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return 2
		}
		if err := launcher.Doctor(*config, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	case "list":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "list does not accept arguments")
			return 2
		}
		names, err := installer.SkillNames(source)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, name := range names {
			fmt.Fprintln(stdout, name)
		}
		return 0
	case "init":
		flags := flag.NewFlagSet("init", flag.ContinueOnError)
		flags.SetOutput(stderr)
		dir := flags.String("dir", ".", "project directory")
		agent := flags.String("agent", "all", "all, agent, codex, claude, or pi")
		preview := flags.Bool("dry-run", false, "preview without writing files")
		force := flags.Bool("force", false, "replace conflicting bundled files")
		if err := flags.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if flags.NArg() != 0 {
			fmt.Fprintln(stderr, "unexpected arguments; use --dir PATH to select a project")
			return 2
		}
		plan, err := installer.NewPlan(source, *dir, *agent, *force)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if *preview {
			for _, entry := range plan.Entries {
				fmt.Fprintf(stdout, "%s %s\n", entry.Action, entry.Path)
			}
			fmt.Fprintf(stdout, "Preview: %d instruction files and %d skills across %d folders; no files written.\n", len(plan.Instructions), len(plan.Skills), len(plan.Targets))
			return 0
		}
		if err := plan.Apply(); err != nil {
			fmt.Fprintf(stderr, "installation failed (it may be partial; resolve the error and rerun): %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Installed orchestrator and %d skills in %s\n", len(plan.Skills), plan.Root)
		for _, instruction := range plan.Instructions {
			fmt.Fprintf(stdout, "  %s\n", instruction)
		}
		for _, target := range plan.Targets {
			fmt.Fprintf(stdout, "  %s\n", target)
		}
		fmt.Fprintln(stdout, "Open or refresh your agent session to load the orchestrator, then provide the authorized scope and test accounts.")
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q; run pentest-harnes --help\n", args[0])
		return 2
	}
}
