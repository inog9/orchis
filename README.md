# pentest-harnes

A Go launcher for a persistent, interactive **Pi pentest orchestrator**. It bundles
the orchestrator, all 21 skills, references, and a code-enforced workflow
controller. Each engagement has an isolated workspace and resumable Pi session.

See the [harness workflow](docs/workflow.md) for Mermaid diagrams covering startup,
sessions, phase ordering, and the verification/reflection/hypothesis loop.

## Install

Requires Go 1.23+ to build, Node.js 22.19+, and Pi 1.5.0+ to run. This repository
is private; authenticate to GitHub first.

```sh
gh repo clone r1pu5u/pentest-harnes
cd pentest-harnes
go install ./cmd/pentest-harnes

# If Pi is not already installed (MCP support is built in):
npm install -g @earendil-works/pi-coding-agent

# Live Fleet inspection, long-context compression, and durable planning:
pi install npm:pi-subagents
pi install npm:billion-context
pi install npm:@xynogen/pix-todo
pi install npm:@juicesharp/rpiv-ask-user-question
```

Use your existing Pi installation if already configured. Provider/model settings
and credentials stay in Pi's normal agent directory (`~/.pi/agent`, or
`PI_CODING_AGENT_DIR`); the harness does not replace them. Configure/login with Pi
before starting an engagement. `pentest-harnes doctor` checks local prerequisites.

Add Go's binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when GOBIN is
unset) to your `PATH`. Alternatively, build a binary at a chosen location:

```sh
go build -o ./bin/pentest-harnes ./cmd/pentest-harnes
```

On Windows, use `pentest-harnes.exe` as the build output filename.

## Run

Run in the directory where you want to keep engagements:

```sh
pentest-harnes
```

Choose **continue an existing session** or **start a new session**. With no saved
sessions, the launcher creates one and opens interactive Pi. Enter your goal,
authorized origins/actions/test window, and test-account details. The orchestrator
records scope and starts work through its controller tools. No target is assumed
from examples embedded in the skills.

For an unattended run, one instruction such as `pentest alfagift.id and the API
in this decompiled app` is enough to start. The lead records the named target,
launches independent workers, verifies evidence and continues across ordinary
worker completions and phase changes. It does not impose a 1–3-hour cutoff;
runtime depends on the work and available accounts. Supply an account only when
an affected authenticated test needs one. `/pentest-agents` and `/pentest-status`
remain available to inspect progress without steering every task.

Scope registration records the supplied target; the lead chooses concrete
exploration, API discovery and review assignments rather than a generic inventory
bootstrap.
The lead has full tools, including bash, and delegates independent work to parallel
Pi processes. `/pentest-agents` and the live widget show actual PIDs, progress,
queued work, and returned reports.

### Click to inspect workers

With `pi-subagents` installed, harness tasks run as native background children
owned by its Fleet UI. Open **`/subagents-fleet`**, then **left-click a worker row**
to see its actual live transcript, current tools, and output. Scroll with the
mouse wheel or `PgUp`/`PgDn`; `x` or `Ctrl+O` toggles tool details. `Esc` closes
the panel. The generated workspace uses Pi fullscreen mode for mouse dispatch.
The persistent FleetView also keeps active work visible below the editor.

`s` can send guidance and `D` stops the selected run. Parent verification remains
separate from worker completion. The harness's required child guard and admission
bindings apply to every native child; raw uncontracted package launches cannot
replace `pentest_delegate`. If the package is absent, the CLI-worker fallback
still runs tasks and `/pentest-agents` reports them, but native Fleet inspection
requires installing `pi-subagents`.

`billion-context` keeps long-running conversation context compressed, and Pix
Todo provides durable execution planning. The generated context configuration
disables the redundant ACP executor while keeping compression. Todo state is
not coverage evidence and cannot advance controller gates.

`@juicesharp/rpiv-ask-user-question` provides visible choice dialogs and terminal
attention for real blocking questions. The lead uses the target already supplied;
missing optional details do not stop independent work. A dismissed dialog pauses
only when the lead records that choice; the legacy `pentest_ask` helper prepares
Continue offline / Pause choices and never uses a hidden input wait.

Batch delegation uses `pentest_delegate({tasks:[...]})`. Eight workers may run
concurrently by default; set `PENTEST_HARNES_MAX_AGENTS=16` for sixteen.

```sh
pentest-harnes --new --name "Staging API"
pentest-harnes sessions
pentest-harnes --session <id-or-unique-prefix>
pentest-harnes --dir /path/to/engagements
pentest-harnes --dry-run
pentest-harnes doctor
```

Session layout:

```text
your-project/.pentest-harnes/sessions/<session-id>/
  session.json                 # session identity/name
  state.json                   # authoritative workflow checkpoint
  events.jsonl                 # workflow transitions
  tool-events.jsonl            # tool completion metadata (no arguments)
  launches/<launch-id>.json    # estimated usage and reference reservations; no spend cutoff
  pi/                          # persistent orchestrator conversation
  specialists/<task-id>/      # PID/process.json, process events, Pi transcript, scratch
  private/                     # pricing, inflight tools, scheduler
    billion-context.json       # session-local compression config, if installed
  runtime/                     # bundled guard, native Fleet backend and controller
  workspace/
    AGENTS.md
    .pi/mcp.json               # native MCP configuration (private credentials)
    .pi/agents/pentest-harness-leaf.md  # full-tool, no-nesting native specialist
    .pi/skills/<skill>/SKILL.md
    private/                   # raw identity/HTTP evidence and secrets
    recon/
    reports/                   # checklist, tasks, hypotheses, phase reports
    <target>_PENTEST_REPORT.md
    <target>_VALIDATION_REASON.md
```

The launcher leaves the surrounding project's instructions and settings alone.
Session directories are private; imported MCP credentials are stored with mode
0600 on POSIX. A nested `.gitignore` excludes session data from Git by default.
Keep `.pentest-harnes/` out of version control. Bundled runtime and
skills inside a session are refreshed when launching the current binary; keep
custom work outside those managed paths. Reports, evidence, checkpoints, Pi
history, and extra files are preserved.

### Long-running workflow and rigid gates

There is **no fixed eight-hour runtime or arbitrary reflection iteration cap**.
An engagement continues while authorized, useful checklist/hypothesis work remains
and the provider remains available. Spending is reported as an estimate. The controller enforces:

- Phase dependencies from setup/exploration through recon, auth, authz, injection,
  fuzzing, deep endpoint fuzzing, gadgets, reflection, evidence, and report.
- Explicit scope, actor/session contracts, unique outputs, request/time/rate
  bounds, and task dependencies before specialist dispatch.
- Offline local review/reflection accepts empty targets and zero request/rate
  budgets. Independent offline review can start ahead of its phase label;
  future live assignments queue instead of failing during setup.
- Scope/run do not spawn generic inventory workers. The lead launches goal-driven
  assignments; ready queued work is dispatched between running lead turns.
- Lead and specialists retain full tools. Verification grants are optional
  replay contracts; normal lead execution needs no grant.
- Eight independent specialists by default, configurable with
  `PENTEST_HARNES_MAX_AGENTS`. Conflicting state and same-session mutations are
  serialized; read-only shared-session work can run in parallel.
- Specialists have their own deadline/parent-liveness watchdog, so an orphaned
  worker stops if its orchestrator exits rather than continuing indefinitely.
- A specialist's return is not completion: the lead verifies the actual report,
  underlying evidence, restoration, and follow-ups before accepting the task.
  Process provenance (recorded PID or a genuine legacy transcript) is required.
- Report parent directories exist before a worker starts. Workers write an early
  substantive report, then update it, preserving partial coverage if interrupted.
- A failed worker does not pause or cancel unrelated specialists. Recoverable lead
  model errors get up to two checkpointed retries before the run pauses; explicit
  user cancellation still stops the workers.
- Every checklist item has an evidence-backed disposition. Blocked items need
  attempts and a concrete client action, and are never rendered as passed.
- Checklist updates accept up to 50 individually evidenced rows per atomic
  `pentest_workflow({action:"checklist",data:{items:[...]}})` call instead of
  one workflow call per row. The lead keeps bash for real checks; leading
  `bash sleep` polling is blocked while workers are running because their native
  completion event wakes the orchestrator.
- Every phase needs a verified delegated reflection task. Coverage changes make
  earlier reflection stale. Valuable hypotheses reopen the appropriate phase.
- Each phase also needs its matching specialist skill; recon requires separate
  recon, JavaScript-analysis, and dependency-confusion coverage.
- New roles reopen exploration, new JS/maps reopen recon, and new authorization
  cells reopen authorization. Missing intermediate roles remain real gaps.
- Actual evidence files are hashed and checked again at gates. Validated
  candidates need request/response/control/identity/impact proof and replay
  evidence. The controller calculates CVSS 3.1 base scores from the supplied vector.
- Final delivery needs both report files, reconciled coverage, candidate verdicts,
  and an exhausted or explicitly blocked hypothesis ledger. Blockers produce
  **limited** delivery, not a full-coverage claim.

Role and authorization policies need source evidence; `UNKNOWN` policy also
limits the coverage claim.

The lead still judges evidence meaning, policy, reproducibility, and report
accuracy. File hashes establish evidence integrity, not vulnerability truth.

### Pi commands

| Command | Purpose |
| --- | --- |
| `/pentest-status` | Current phase, task/checklist summary, and launch spending. |
| `/pentest-agents` | Actual specialist PIDs, status/progress, queued-phase readiness, and reports. |
| `/subagents-fleet` | Native live roster: click/select a worker for its transcript, tools, output, and controls. |
| `/pentest-integrations` | Show active Fleet, compression, todo and estimate-mode integrations without a model call. |
| `/pentest-run` | Continue authorized work from the saved state. |
| `/pentest-pause` | Pause automatic continuation and interrupt active work. |
| `/pentest-pricing INPUT OUTPUT CACHE_READ CACHE_WRITE` | Confirm model prices in USD per million tokens. |
| `/mcp` | Pi's built-in MCP connection/configuration UI. |

Ordinary turns auto-continue once the workflow is running. User aborts, provider
errors, missing authorization/accounts, ambiguous side effects, and other real
exceptions pause work. Session-local settings disable automatic model/provider
retries so each subsequent attempt passes the workflow and reporting hooks. Resuming restores
the conversation and checkpoint; use
`/pentest-run` after reconciling any interrupted tasks. Never blindly repeat a
possibly successful mutation. A stale `launch.lock` after an ungraceful launcher
crash must be removed only after confirming that launcher and Pi have exited; the
controller still requires interrupted-work reconciliation afterward.
An empty partial report is not accepted as returned evidence. Explicit
`reconcile_task` retries preserve partial files and queue a fresh bounded task;
pending dependents are rebound to the replacement and its report. Scope edits
during setup remain in setup rather than skipping its completion gates.
The launcher requires the workflow extension to load successfully; it does not
silently fall back to an ungated Pi session.

### Assignments and verification

An offline specialist needs only a concrete question and skill:

```json
{
  "task": {
    "id": "local-review",
    "question": "Review supplied local artifacts and identify setup gaps without target requests",
    "skill": ".pi/skills/pentest-exploration/SKILL.md",
    "network": false
  }
}
```

The effective contract defaults to the current phase, `reports/<id>.md`, a local
actor, read-only work, 10 minutes, and zero network requests. Live assignments
require exact targets and use bounded network defaults (20 requests, 1 req/s,
10 minutes, anonymous actor). Authenticated actors require explicit verified
identity/session evidence; mutations require a restoration plan. Independent
offline reviews may run ahead of the phase label. Future live tasks return
`queued` and keep their IDs for later dispatch.

After a specialist returns, the lead can call `pentest_workflow` with
`begin_verification`, its task ID and a reason. The grant has bounded tool calls,
requests and time within the task's limits; offline grants cannot make managed
network requests. `verify_task` still requires real report/raw evidence and
follow-up consideration. Scope/phase changes, pause, and successful verification
clear the grant. Lead reads, scope capture, planning and ledger synthesis remain
available without an execution grant.

### Spending and traffic

Each invocation starts a fresh **spending estimate**, shared across the
orchestrator and guarded specialists. The user selected compression with
**estimate-only accounting and no hard monetary cutoff**. `$15` remains a ledger
reporting reference, not an execution limit. Previous launch ledgers remain intact.
Usage reported by Pi is priced with configured token rates; unavailable prices
are marked unknown instead of blocking work. `/pentest-pricing` can improve the
estimate and remembers confirmed rates per session/model.

`billion-context` may issue internal compression/preflight requests not fully
represented in Pi's assistant usage. Therefore the estimate is not a guaranteed
actual bill or end-to-end spending cap. Use `/acp-cache` for the compression
package's own usage/cache reporting. Legacy strict-mode ledgers remain supported
for explicit configurations and regression tests; new launches use estimate mode.

#### DeepSeek V4.1 Flash through OpenCode Go

Use Pi provider `opencode-go` and model `deepseek-v4.1-flash`. The supplied
peak-rate budget prices per million tokens are:

| Input | Output | Cached read | Separate cache write |
| --- | --- | --- | --- |
| $0.30 | $1.20 | $0.006 | $0 |

For an existing session, confirm these rates inside Pi with:

```text
/pentest-pricing 0.30 1.20 0.006 0
```

If a continued session restores a different model, select
`/model opencode-go/deepseek-v4.1-flash` first.

For new sessions, set the model's `cost` in Pi's `models.json` to
`{"input":0.30,"output":1.20,"cacheRead":0.006,"cacheWrite":0}` and choose it
as the default in Pi's `settings.json`:

```json
{
  "defaultProvider": "opencode-go",
  "defaultModel": "deepseek-v4.1-flash"
}
```

An [example models.json](docs/examples/pi-opencode-go.models.json) includes the
model definition, pricing, and DeepSeek compatibility settings. Merge its
provider entry into your existing `~/.pi/agent/models.json`; the example reads
the key from `OPENCODE_API_KEY`.

If Pi's built-in catalog only lists the older `deepseek-v4-flash`, add the V4.1
model using the current OpenAI-compatible base URL
`https://opencode.ai/inference/go/openai/v1`. Its catalog limits are 1,000,000
context tokens and 384,000 output tokens. Configure authentication through Pi's
OpenCode Go login/key settings or a private credential reference.

OpenCode Go uses these token prices to meter the plan's allowance. The harness's
$15 reporting reference is separate local estimated usage. The fixed supplied peak
rates are conservative during off-peak periods; they do not track remaining
monthly/weekly/5-hour plan allowances.

References: [OpenCode Go pricing](https://opencode.ai/docs/go/) and
[current model metadata](https://opencode.ai/inference/go/v1/models).

`pentest_http` schedules individually captured requests at a maximum of **50
requests/second aggregate** across specialists. Native MCP calls, including calls
nested inside codemode, are also paced, with per-task request counters. Stricter
skill limits and stop rules still apply. Captured HTTP calls do not follow
redirects or retry automatically.
The single-request helper has a 30-second timeout and an 8 MiB response-capture
limit; use the proxy/static-collection workflows for larger artifacts.

**The lead and specialists retain full tool access, including bash.**
Shell commands, browser actions, requests
issued internally by an MCP server, and external processes can issue requests
outside this scheduler. The harness is not a network sandbox; those tools must
use their own bounded rates.

### Integrations

The launcher imports enabled OpenCode MCP entries plus the explicitly requested
Burp, Caido, and Textbee entries even when marked disabled in OpenCode. This
affects only the session's private copy; global OpenCode/Pi configuration is not
changed. The harness writes a private, native `.pi/mcp.json` in the engagement
workspace. Pi connects enabled servers in the background when the session starts.
Specialists use that same workspace configuration.

The launcher loads Pi's built-in MCP, codemode, and tool-search extensions.
Tools have native `mcp__<server>__<tool>` names. Imported OpenCode servers use
`codemode` exposure by default to avoid loading every tool definition into the
model context; `tool_search` can discover tools for direct calls. Existing native
Pi exposure preferences are retained. The scope/window gate, request counter,
and shared scheduler also apply to underlying calls made from codemode scripts.

- OpenCode lookup: `--opencode-config PATH`, then `OPENCODE_CONFIG`, then
  `$XDG_CONFIG_HOME/opencode/opencode.jsonc` / `opencode.json` (default
  `~/.config`). JSONC comments/trailing commas and `{env:...}` / `{file:...}`
  references are supported. Literal credentials remain in the private copy.
- Existing Pi global MCP definitions are merged before the OpenCode entries.
  Native exposure, OAuth, enabled state, timeouts, and per-tool exposure settings
  are retained. Common legacy extension options are converted to native tool
  exposure or authorization headers.
- Interceptor drives browser work using the bundled `interceptor`,
  `interceptor-browser`, and `interceptor-research` skills and their reference/
  workflow files. Burp/Caido MCP provide HTTP replay/evidence; their integration
  skills are loaded from your existing skill folders.
- Gmail verification uses `gog`; authenticator challenges use `totp-cli`; phone
  verification uses Textbee. Only engagement-authorized accounts/phones are used.
- MCP authentication and connection management use Pi's built-in `/mcp` UI and
  `pi mcp` CLI. Native HTTP transport is streamable HTTP; a legacy SSE service
  needs a streamable-HTTP endpoint or a stdio proxy, such as the existing Burp
  proxy command.

For an older Pi setup, remove the replacement extension with
`pi remove npm:pi-mcp-adapter` so Pi's built-in `/mcp` can load. On the next harness
launch, older managed `private/mcp.json` copies migrate to `workspace/.pi/mcp.json`.

`doctor` checks executable/configuration availability without invoking inbox,
SMS, or target tools. It labels configured MCP services as **connection not
tested**; live connectivity and authentication are verified during their use.

### Programmatic mode

```sh
pentest-harnes --new --rpc          # Pi JSON-line RPC on stdin/stdout
pentest-harnes --new --print --prompt "<authorized engagement instructions>"
```

In RPC mode, launcher diagnostics go to stderr so stdout remains JSON-lines.
Both modes use the same persistent controller, spend reporting, and specialist
sessions. Non-interactive exceptions are returned as blockers rather than guessed.

## Install files without launching Pi

`init` remains available for instructions/skills-only installation:

```sh
# Preview without creating files or directories
pentest-harnes init --dry-run

# Select another project or a single agent folder
pentest-harnes init --dir /path/to/project
pentest-harnes init --agent claude
pentest-harnes init --agent codex
pentest-harnes init --agent pi

# Update bundled files, replacing local edits only when explicitly requested
pentest-harnes init --dry-run --force
pentest-harnes init --force

# Inspect the binary
pentest-harnes list
pentest-harnes version
```

Re-running keeps identical files. A differing bundled file stops initialization
before writes unless `--force` is supplied. This includes existing `AGENTS.md`
and selected `CLAUDE.md` instructions: review conflicts before using `--force`,
which replaces those files. Other skills, custom extra files, and agent settings
are preserved. Symlink destinations
inside the selected project are rejected. Files are copied independently: an
I/O failure can leave a partial installation; resolve the error and rerun.

The default `init --agent all` installs root `AGENTS.md`, `CLAUDE.md` (imports
`AGENTS.md`), and skills under `.agent/skills`, `.agents/skills`, `.claude/skills`,
and `.pi/skills`. Single-agent selections use their corresponding folder;
`CLAUDE.md` is created only for `all` or `claude`. Links in `AGENTS.md` are adjusted
to the installed skill location. This mode installs instructions only; use the
Pi launcher for the code-enforced controller.

## Included skills

Skill sources live under [`skills/`](skills/), one directory per skill:

```text
skills/
  interceptor/SKILL.md
  interceptor-browser/SKILL.md
  interceptor-research/SKILL.md
  pentest/SKILL.md
  pentest-exploration/SKILL.md
  pentest-recon/SKILL.md
  ...
  gog-gmail/SKILL.md
  totp-cli/SKILL.md
```

| Skill | Purpose |
| --- | --- |
| `interceptor` | Interceptor surface routing and background-first session/focus rules |
| `interceptor-browser` | Browser automation, page/network inspection, screenshots, references, and workflows |
| `interceptor-research` | Deep browser research, source ledgers, verification, and stopping rules |
| `pentest` | Shared rules, workflow, and checklist |
| `pentest-exploration` | Initial exploration, test-account setup, and triage |
| `pentest-recon` | Routes, API inventory, JavaScript, and business flows |
| `wordpress-pentest` | Frontend plugin discovery, calibrated slash/readme oracles, bounded 10k/132k enumeration, all-plugin source review, and live proof |
| `pentest-js-analysis` | Delegated lazy-JS/gau-first archive collection and full-corpus jsluice/direct review |
| `pentest-dependency-confusion` | Dependency-confusion discovery from collected JS/source maps, independent npxconfuse coverage and optional exposed manifests |
| `pentest-auth` | Authentication and account lifecycle |
| `pentest-authz` | Object, function, and tenant authorization |
| `pentest-injection` | Injection and unsafe parsing |
| `pentest-fuzzing` | Bounded discovery and fuzzing |
| `pentest-endpoint-fuzz-full` | Seeded deep endpoint discovery and five-run loop |
| `pentest-gadgets` | Gadget definitions, proof requirements, and research notes |
| `pentest-reflection` | Coverage reconciliation and valuable follow-up judgment |
| `pentest-evidence` | Evidence capture, verdicts, and scoring |
| `pentest-report` | Client report and internal validation ledger |
| `gog-gmail` | Gmail command guidance for the authorized test inbox |
| `totp-cli` | Authenticator TOTP codes for an authorized account's 2FA challenge |
| `pentest-phone-verification` | Phone confirmation and SMS 2FA using the authorized Textbee-connected test phone |

The Interceptor skill directories are self-contained snapshots copied from the
OpenCode installation at `~/.config/opencode/skills/`, including their references
and workflows. They are embedded in the Go binary and installed with the suite.

## Tools used by the skills

The initializer installs instructions and references. Set up the tools needed
by the chosen workflow separately:

- Interceptor CLI for browser work; the Interceptor skills are bundled.
- ffuf 2.1, Python 3.9+, curl and unzip for WordPress discovery/source retrieval.
  `wordpress-pentest` bundles the unchanged 10,000/132,211-entry plugin lists,
  an offline preparer and a bounded ffuf supervisor; the supervisor requires
  Linux/macOS. The skill's references cover calibration, source versions,
  unauthenticated-first full review and live validation.
  Bulk ffuf runs directly; Burp/Caido handles selected candidate replays and
  evidence instead of the dictionary traffic.
- Burp or Caido with its configured proxy/replay integration for HTTP evidence.
- jxscout for JavaScript analysis when required.
- gau CLI first for historical URL discovery, followed by curl for Wayback CDX
  and reachable static JS/map downloads; the recon skill delegates collection
  gaps and review shards with per-file coverage.
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
- Specialist delegation is supplied by the Pi runtime's `pentest_delegate` tool.
- Textbee MCP with the authorized test phone when SMS verification is required.

No mailbox is assumed or authenticated by `init`; supply the engagement's inbox
before using account-creation or verification flows. Initialization itself does
not start a pentest or make requests to a target.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
node --test tests/pi/*.test.mjs
python3 -m unittest discover -s tests -v
go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" \
  -o ./bin/pentest-harnes ./cmd/pentest-harnes
```

The real-Pi smoke tests are opt-in and use a local mock model, with no paid API
requests or target testing:

```sh
go build -o /tmp/pentest-harnes ./cmd/pentest-harnes
PENTEST_HARNES_TEST_BIN=/tmp/pentest-harnes \
PENTEST_HARNES_TEST_PI="$(command -v pi)" \
node --test tests/pi/pi-smoke.test.mjs
```

To exercise native Fleet workers and actual transcript inspection, also set
`PENTEST_HARNES_TEST_SUBAGENTS_DIR="$HOME/.pi/agent/npm/node_modules/pi-subagents"`.
The local-model fixture disables compression bootstrap and uses no real provider
credentials or target requests. The separate opt-in user-profile startup check
verifies Fleet/Todo registration and `/acp` proxy readiness without a model call.

The real-Pi fixture exercises native MCP calls from the lead and specialist,
codemode-nested calls, pre-scope blocking, and per-task request budgets.
Set `PENTEST_HARNES_TEST_USER_CONFIG=1` as well to check loading your existing Pi
and native MCP configuration without invoking model/inbox/SMS tools. Enabled
servers initialize in the background during that startup check. Controller tests
cover phase gates, stale evidence, role reopening, hypothesis exhaustion,
interrupted mutations, limited delivery, shared budgets, and request pacing.

Edit `AGENTS.md`, `pi-runtime/`, or the skill folders under `skills/` and
rebuild to include changes.
Generated installations, reports, and binaries are excluded from version control.

## License

MIT. Research links credit their original authors; referenced third-party
articles retain their own rights.
