package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"

	"github.com/r1pu5u/pentest-harnes/internal/installer"
)

const help = `pentest-harnes — install bundled pentest skills into a project

Usage:
  pentest-harnes init [--dir PATH] [--agent all|agent|codex|claude] [--dry-run] [--force]
  pentest-harnes list
  pentest-harnes version

init defaults to the current directory and installs into:
  .agent/skills    requested compatibility folder
  .agents/skills   Codex project skills
  .claude/skills   Claude Code project skills

Existing identical files are kept. Different files require --force.
The binary bundles all skills and references; initialization needs no network.
`

// Run executes the CLI without exiting, allowing isolated end-to-end tests.
func Run(args []string, source fs.FS, version string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, help)
		return 0
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, help)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "pentest-harnes %s\n", version)
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
		agent := flags.String("agent", "all", "all, agent, codex, or claude")
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
			fmt.Fprintf(stdout, "Preview: %d skills across %d folders; no files written.\n", len(plan.Skills), len(plan.Targets))
			return 0
		}
		if err := plan.Apply(); err != nil {
			fmt.Fprintf(stderr, "installation failed (it may be partial; resolve the error and rerun): %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Installed %d skills in %s\n", len(plan.Skills), plan.Root)
		for _, target := range plan.Targets {
			fmt.Fprintf(stdout, "  %s\n", target)
		}
		fmt.Fprintln(stdout, "Open or refresh your agent session, then invoke the pentest skill.")
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q; run pentest-harnes --help\n", args[0])
		return 2
	}
}
