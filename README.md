# pentest-harnes

A Go CLI that installs this pentest skill suite into a project's local agent
folders. Skills and references are embedded in the binary, so `init` works offline
and does not need the source checkout beside it.

## Install

Requires Go 1.23 or newer. This repository is private; authenticate to GitHub first.

```sh
gh repo clone r1pu5u/pentest-harnes
cd pentest-harnes
go install ./cmd/pentest-harnes
```

Add Go's binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when GOBIN is
unset) to your `PATH`. Alternatively, build a binary at a chosen location:

```sh
go build -o ./bin/pentest-harnes ./cmd/pentest-harnes
```

On Windows, use `pentest-harnes.exe` as the build output filename.

## Initialize a project

Run in the project where you want the skills available:

```sh
pentest-harnes init
```

Default output:

```text
your-project/
  .agent/skills/<skill-name>/SKILL.md
  .agents/skills/<skill-name>/SKILL.md
  .claude/skills/<skill-name>/SKILL.md
```

Each skill's references are copied with it, preserving links between skills.
`.agent` is the requested compatibility folder. Codex discovers project skills
in [`.agents/skills`](https://learn.chatgpt.com/docs/build-skills#where-codex-loads-local-skills);
Claude Code uses [`.claude/skills`](https://code.claude.com/docs/en/skills#choose-where-skills-load).

```sh
# Preview without creating files or directories
pentest-harnes init --dry-run

# Select another project or a single agent folder
pentest-harnes init --dir /path/to/project
pentest-harnes init --agent claude
pentest-harnes init --agent codex
pentest-harnes init --agent agent

# Update bundled files, replacing local edits only when explicitly requested
pentest-harnes init --dry-run --force
pentest-harnes init --force

# Inspect the binary
pentest-harnes list
pentest-harnes version
```

Re-running keeps identical files. A differing bundled file stops initialization
before writes unless `--force` is supplied. Other skills, custom extra files,
`AGENTS.md`, `CLAUDE.md`, and agent settings are preserved. Symlink destinations
inside the selected project are rejected. Files are copied independently: an
I/O failure can leave a partial installation; resolve the error and rerun.

Open or refresh the agent session after installation. Invoke `$pentest` in Codex
or `/pentest` in Claude Code, supplying the engagement's authorized scope and
test accounts. Specialist skills can be invoked by their own names.

## Included skills

| Skill | Purpose |
| --- | --- |
| `pentest` | Shared rules, workflow, and checklist |
| `pentest-exploration` | Initial exploration, test-account setup, and triage |
| `pentest-recon` | Routes, API inventory, JavaScript, and business flows |
| `pentest-js-analysis` | Step-by-step jsluice analysis with bundled Tree-sitter source/sink queries |
| `pentest-dependency-confusion` | Dependency-confusion discovery from collected JS/source maps, independent npxconfuse coverage and optional exposed manifests |
| `pentest-auth` | Authentication and account lifecycle |
| `pentest-authz` | Object, function, and tenant authorization |
| `pentest-injection` | Injection and unsafe parsing |
| `pentest-fuzzing` | Bounded discovery and fuzzing |
| `pentest-gadgets` | Gadget definitions, proof requirements, and research notes |
| `pentest-evidence` | Evidence capture, verdicts, and scoring |
| `pentest-report` | Client report and internal validation ledger |
| `gog-gmail` | Gmail command guidance for the authorized test inbox |
| `totp-cli` | Authenticator TOTP codes for an authorized account's 2FA challenge |
| `pentest-phone-verification` | Phone confirmation and SMS 2FA using the authorized Textbee-connected test phone |

## Tools used by the skills

The initializer installs instructions and references. Set up the tools needed
by the chosen workflow separately:

- Interceptor CLI and the `interceptor-browser` skill for browser work.
- Burp or Caido with its configured proxy/replay integration for HTTP evidence.
- jxscout for JavaScript analysis when required.
- jsluice and Python 3.9+ for offline JavaScript analysis. `init` includes the
  query collection under `<agent-folder>/skills/pentest-js-analysis/tree-sitter-query/`
  and the runner under that skill's `scripts/` directory. The original root-level
  `tree-sitter-query` collection now lives inside this skill so no separate copy
  is needed after installation.
- gog CLI with an explicitly authorized Gmail account for email verification.
- npxconfuse, when available, for registry-name checks alongside independent
  collected-JS analysis in `pentest-dependency-confusion`. No repository,
  package installation or exposed manifest is required for that workflow.
- [totp-cli](https://github.com/yitsushi/totp-cli) with the relevant authorized
  token when the website requires authenticator-based 2FA. The skill checks the
  installed command help before use; `init` does not install or enroll tokens.
- Agent delegation support for phases that require subagents.
- Textbee MCP with the authorized test phone when SMS verification is required.

No mailbox is assumed or authenticated by `init`; supply the engagement's inbox
before using account-creation or verification flows. Initialization itself does
not start a pentest or make requests to a target.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
python3 -m unittest discover -s tests -v
go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" \
  -o ./bin/pentest-harnes ./cmd/pentest-harnes
```

Edit the skill folders at the repository root and rebuild to include changes.
Generated installations, reports, and binaries are excluded from version control.

## License

MIT. Research links credit their original authors; referenced third-party
articles retain their own rights.
