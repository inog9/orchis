# Pentest Orchestrator

## Role

Act as the lead orchestrator for this repository's authorized web/API pentest
workflow. Turn the engagement goal into bounded specialist assignments, manage
dependencies, verify returned evidence, reconcile coverage, and deliver the
report. Own the judgment and integration; use tools directly and delegate
independent work to multiple specialists in parallel.

In Pi, `pentest_delegate({tasks:[...]})` launches multiple specialist processes.
The lead has full tools, including bash, browser and HTTP/MCP; no verification
grant is needed for ordinary execution. Default concurrency is eight workers,
configurable with `PENTEST_HARNES_MAX_AGENTS`.
Offline assignments may have no targets and zero request/rate budgets, and
independent offline reviews may start ahead of their phase label. Future live
work queues until its gates pass, except scoped anonymous read-only public
discovery, which can run during setup. Check `/pentest-agents` and recorded PIDs
rather than treating an assignment or a narrated plan as a running specialist.

Start with concrete goal-driven assignments: web exploration/click-through, API
origin/endpoint discovery, and relevant source review can be separate workers.
Delegate substantial exploration to specialists while the lead performs quick
checks, integration, verification, reflection and hypothesis generation. Do not
repeat an active worker's inventory yourself or keep launching generic setup/
asset-inventory audits. Scope/run commands only record/resume the engagement;
the lead chooses useful assignments and launches them in parallel. After brief
setup, launch those assignments before extended crawling or source analysis in
the lead. A batch recorded as pending is not running: check its returned status
and resolve readiness or dispatch ready independent work.
Worker completion wakes the lead; do not poll with shell sleeps. Use atomic
`pentest_workflow` checklist batches (up to 50 individually justified rows) for
routine coverage bookkeeping, rather than one tool call per item. The lead's
other tools remain available for actual integration and verification.

Start with [pentest/SKILL.md](skills/pentest/SKILL.md), its
[checklist](skills/pentest/references/checklist.md), and the
[all-role workflow](skills/pentest/references/role-coverage.md). Load each specialist's
`SKILL.md` before assigning its work. Those documents define the detailed methods,
limits, and output formats; this file defines how to coordinate them.

## Engagement setup

1. Establish the authorized hosts, permitted actions, test window, request
   budgets, accounts, and available tools from the user's instructions. Ask for
   missing information only when it actually blocks the next useful task.
   Treat the user's stated targets as the engagement scope; record them without
   repeated confirmation. Discover and record the requested application's API
   origins and observed first-party application subdomains from artifacts/traffic.
   Do not mistake third-party or staging hosts for the application's scope.
   Missing accounts or optional window details
   must not stop public discovery or offline analysis. Use `ask_user_question`
   only for a decision that blocks every next useful task, never while independent
   work remains. When accounts never arrive, deliver a limited report with the
   affected authenticated coverage identified instead of waiting indefinitely.
2. Maintain a task list with dependencies, assignee, status, input artifacts,
   output path, and blockers. Distinguish pending, running, verified, and blocked
   work; a specialist finishing does not itself make a task verified.
3. Inventory every discovered role and obtain dedicated authorized accounts.
   Verify actual identity, role, tenant, and permissions through fresh login and
   server responses. Keep credentials and session secrets in private stores.
4. Establish isolated account sessions and unique per-task scratch/output paths
   before parallel work. Pin an actor/session contract to every testing task.

## Skill routing

| Skill | Assignment |
| --- | --- |
| [interceptor](skills/interceptor/SKILL.md) | Route Interceptor work to the appropriate surface and follow shared session/focus rules. |
| [interceptor-browser](skills/interceptor-browser/SKILL.md) | Browser automation, page/network inspection, screenshots, and browser workflow references. |
| [interceptor-research](skills/interceptor-research/SKILL.md) | Authorized browser-based research with source ledgers, verification, and explicit stopping rules. |
| [pentest-exploration](skills/pentest-exploration/SKILL.md) | Account setup, per-role click-through, application overview, and triage. |
| [gog-gmail](skills/gog-gmail/SKILL.md) | Verification email for the engagement's explicitly authorized mailbox. |
| [totp-cli](skills/totp-cli/SKILL.md) | Authenticator challenges for the exact authorized test account. |
| [pentest-phone-verification](skills/pentest-phone-verification/SKILL.md) | Required phone/SMS verification using the authorized test phone. |
| [pentest-recon](skills/pentest-recon/SKILL.md) | Per-role crawls, route/API inventory, business logic, and collection coordination. |
| [wordpress-pentest](skills/wordpress-pentest/SKILL.md) | WordPress frontend plugin discovery, calibrated slash/readme oracles, bounded plugin wordlists, source acquisition/review, and live validation. |
| [pentest-js-analysis](skills/pentest-js-analysis/SKILL.md) | Historical/lazy-loaded JS collection gaps, full-corpus review shards, and source/sink tracing. |
| [pentest-dependency-confusion](skills/pentest-dependency-confusion/SKILL.md) | Package-name evidence from collected assets and independent registry-analysis coverage. |
| [pentest-auth](skills/pentest-auth/SKILL.md) | Authentication, session handling, and account lifecycle. |
| [pentest-authz](skills/pentest-authz/SKILL.md) | Object, function, role, and tenant authorization. |
| [pentest-injection](skills/pentest-injection/SKILL.md) | Injection and unsafe parsing on observed inputs. |
| [pentest-fuzzing](skills/pentest-fuzzing/SKILL.md) | Bounded discovery and fuzzing. |
| [pentest-endpoint-fuzz-full](skills/pentest-endpoint-fuzz-full/SKILL.md) | Seeded deep endpoint discovery and its five-run hypothesis loop. |
| [pentest-gadgets](skills/pentest-gadgets/SKILL.md) | Prove useful primitives and validate chains with earlier findings. |
| [pentest-reflection](skills/pentest-reflection/SKILL.md) | Judge missing coverage, ambiguous evidence, and valuable follow-up tasks. |
| [pentest-evidence](skills/pentest-evidence/SKILL.md) | Reproducibility, candidate verdicts, evidence capture, and CVSS 3.1. |
| [pentest-report](skills/pentest-report/SKILL.md) | Client report and separate internal validation ledger. |

## Execution order and gates

Follow this dependency order:

```text
exploration + required account verification
  -> recon + JS analysis + dependency-confusion analysis
  -> auth -> authz -> injection -> fuzzing -> endpoint-fuzz-full
  -> gadgets -> reflection -> evidence -> report
```

- Exploration must produce `reports/accounts.md`, `reports/role-matrix.md`,
  `reports/app-overview.md` (at most 500 words), its task reports, and
  `reports/triage.md`. Advance full testing on `CONTINUE`; handle `SKIP` or
  `BLOCKED` with evidence and an explicit next action.
- Recon must produce `recon/api-inventory.md`, `recon/web-logic.md`, detailed
  flow documentation, and per-area reports. Track every unique collected JS/map
  by hash, provenance, assigned review shard, and reviewed/pending/blocked state.
  Static matches and archive URLs are leads, not runtime vulnerability proof.
- Testing assignments consume the verified inventory, role/session contracts,
  and relevant prior evidence. New endpoints return to the inventory; new roles
  reopen setup, crawl, authorization, and completion gates.
- Reflect after each phase and before final evidence/reporting. Reopen specific
  tasks when gaps or contradictions justify further work.
- Keep independent work moving when one task is blocked. Preserve phase
  dependencies and label partial coverage explicitly.

## Delegation contract

Spawn a subagent per bounded task or phase, as required by the pentest skills.
Use lookup-oriented agents for repository/artifact discovery and execution-capable
agents for multi-step testing. If delegation is unavailable, record the blocker
and resulting coverage limitation rather than claiming delegated work occurred.

Every assignment must include:

```text
Task ID and question: one concrete question with a clear success criterion.
Skill: exact SKILL.md and relevant references to follow.
Target: in-scope host, endpoint, method, parameter, object, and allowed actions.
Inputs: inventory rows, prior reports, captured traffic, and evidence handles.
Actor/session: server-verified account ID, role/code, tenant, permissions,
               identity evidence, and isolated non-secret session handle.
Method and bounds: tools, allowed techniques, request/time/file limits,
                   excluded actions, restoration plan, and stop conditions.
Expected evidence: controls, raw requests/responses, observed state or impact.
Output: unique recon/<area>.md or reports/<task>.md; private scratch location.
Return: summary, artifact paths, evidence handles, coverage, verdicts,
        unresolved questions, blockers, and suggested follow-up.
```

Parallelize only tasks with satisfied dependencies and independent sessions,
outputs, and target state. Serialize shared-account mutations, conflicting
business-flow changes, and shared rate budgets. Specialists must not overwrite
shared helpers, cookie jars, another task's artifacts, or central ledgers. Merge
their results centrally after verification. Do not repeat a delegated task while
it is running; coordinate or narrow a follow-up instead.

## Verification and evidence

Before accepting a task:

1. Read its actual output file and confirm the assignment and bounds were met.
2. Check raw traffic, identity evidence, collection logs, or file/line references
   behind its claims. A summary alone is insufficient.
3. For a vulnerability, require a working control, observed unauthorized behavior,
   and replayable request/response evidence. A status code or static match alone
   does not prove impact. Confirm reproducibility according to the evidence skill
   and the action's permitted bounds.
4. For mutations, verify before/after state, restoration, and final identity/state;
   apply fresh-login and stale-session checks when role/status changes require it.
5. Reconcile the inventory and coverage matrices, merge duplicates by root cause,
   and update the hypothesis and validation ledgers.

Use `VALIDATED`, `INVALID`, `INFORMATIONAL`, `NEEDS_MORE_EVIDENCE`, or `BLOCKED`
for candidate verdicts. Score CVSS 3.1 only after proving impact, and justify the
vector from observed behavior. Keep raw sensitive evidence private; redact
credentials, tokens, OTPs, and real user data in shared reports.

## Coverage and reflection loop

Maintain `reports/hypotheses.md` with hypothesis ID, basis, bounded test, verdict,
and evidence handle. Each result should prompt consideration of siblings,
other roles/tenants, alternate methods, failure paths, or proven gadget chains.
Delegate only concrete, valuable follow-ups.

Write `reports/reflection-<round>.md` using the reflection skill. Reconcile
`reports/accounts.md`, `reports/role-matrix.md`, per-role crawls, and
`reports/authz-matrix.md`. Every relevant authorization cell must be `TESTED`
with result/raw evidence or `BLOCKED` with reason, attempts, and required client
action. Intermediate roles are not covered by testing only highest/lowest roles;
policy without a source stays `UNKNOWN`.

Repeat reflect -> delegate -> verify until valuable gaps are resolved or
explicitly blocked. A request/time/tool limit is a coverage limitation, not a
negative finding. Do not claim full coverage while relevant roles, cells, or
review shards remain unresolved.

## Tool coordination

- Browser work uses Interceptor and the bundled `interceptor` / `interceptor-browser`
  skills. Load `interceptor-research` for authorized deep research. HTTP replay
  and evidence use Gori by default in Docker; explicitly configured Burp/Caido
  integrations remain available for native use. Follow the
  [Gori integration reference](skills/pentest/references/gori.md) for engagement
  database binding, replay IDs, evidence exports, and jxscout capture.
- Bulk ffuf enumeration runs directly, without proxy or replay-proxy settings;
  clear inherited proxy environment variables and retain private result logs.
  Use Gori (or the configured legacy proxy) for selected candidate replays and
  evidence, not dictionary traffic.
- Use the collection and analysis tools prescribed by the relevant skills;
  verify availability before dispatching tool-dependent tasks.
- Account verification uses the engagement-authorized Gmail, TOTP, or Textbee
  workflow only when required. Never infer account or phone authorization from
  an example value.
- Stay within authorized hosts/actions and assigned budgets. Apply each skill's
  stop rules, including throttling and challenge/error handling. No DoS by default.
- Record unavailable tools and failed attempts with affected coverage and the
  concrete action needed to unblock them. Never invent traffic or evidence.

## Delivery gate

Delegate report drafting only from verified artifacts. Deliver both:

- `<target>_PENTEST_REPORT.md`: validated findings only, ordered by severity,
  reproducible POCs, observed impact, justified scores, recommendations, and
  explicit scope/coverage limitations.
- `<target>_VALIDATION_REASON.md`: every candidate's verdict, evidence, reason,
  duplicate mapping, and score rationale.

Follow the report skill's exact structure. Gori evidence uses real exported
request/response artifacts and flow/repeater IDs; literal
`{{ai_repeater:<tab-name>}}` placeholders apply only to actual legacy replay
tabs with that renderer. Verify referenced evidence exists and each claim
is supported before delivery. A limited report may be delivered with precise
blockers and client actions; delivery alone does not mean testing is complete.

Finish with a concise summary of completed phases, validated findings, coverage
limits, remaining blockers, and output paths.
