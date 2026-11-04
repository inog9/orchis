# Scan windows and emergency stop

Administrators can set a weekly scan window on an organisation and stop scans on
that organisation. Settings contains the global emergency stop. Analysts can see
the policy and its hold banner, but cannot change it. Viewers have no scan-control
access. The API repeats these permission checks in the service layer.

A window uses an IANA timezone, ISO weekdays (Monday 1 to Sunday 7), and local
24-hour start/end times. Start is included; end is excluded. An end before the
start belongs to the following day; the chosen weekday is the starting day. Both
occurrences of a repeated daylight-saving hour follow the same local window;
nonexistent local times do not extend the window. An empty window permits scans
at any time. Equal start/end times are rejected; remove the window for unrestricted
scanning. No automatic scans are scheduled.

The controls apply to network scans through Scan, Atomic, Chains and the retained
assessment API. Endpoint inventory collection is separate. Existing scope checks,
authorization, target confirmation and injector scope validation remain required.
The server checks the window/hold at creation, execution claim, heartbeat, result
acceptance and worker recovery. An interrupted result cannot establish a clean
scan or a resolved finding. Completed executions and report snapshots are retained. The scan worker finalizes
interrupted assessments with a canceled snapshot in the same transaction; the
emergency stop cancels jobs immediately without exposing a terminal unfrozen scan.

Emergency stop is durable and idempotent. It cancels waiting scan jobs and marks
live network executions canceled with the stopping actor, rather than failed or
safe. The injector checks cancellation every two seconds and kills its scanner
process group. Resume permits new scans only; it does not restart interrupted jobs.
A global hold takes precedence over an organisation resume. When moving inventory,
both the frozen scan organisation and the current asset organisation must permit
execution. An unassigned asset remains subject to the global hold.

Migration 31 adds tenant-isolated controls and freezes the organisation separately
on targets and executions. A tenant transaction advisory lock is acquired before
scan/job/execution locks so launch and stop cannot cross. Workers re-evaluate the
durable policy after restart. A closing or newly restricted window cancels active
work; no in-memory timer is the source of truth. The scan worker checks each second;
claim, heartbeat and result gates also enforce expiry independently.

## API

- `GET /api/v1/scan-control`: global state, Admin only.
- `POST /api/v1/scan-control/stop` and `/resume`: global Admin controls.
- `GET /api/v1/organizations/{id}/scan-control`: state for Admin/Analyst.
- `PUT /api/v1/organizations/{id}/scan-control`: Admin window change.
- `POST /api/v1/organizations/{id}/scan-control/stop` and `/resume`: Admin controls.

Window changes and resume require the returned `version` (zero for defaults);
stale versions return 409. Emergency stop deliberately takes priority over CAS.
Repeated stops preserve the original actor and do not duplicate the action audit.
Every policy change is audited, and stopped executions receive a durable decision
log and `scan.policy_stop` audit with its reason code. Server clocks can be injected
for deterministic boundary, restart and late-result tests.

`e2e/flow20.mjs` runs real Nmap and Nuclei against slow loopback fixtures and verifies
that the actual scanner PIDs disappear after stop. It also verifies queued work,
durable holds, resume, global UI, axe and dialog geometry at 1280/834/390 pixels.
`e2e/mutation-scan-controls.py` builds each mutant in an isolated module, requires a
related assertion failure, restores the baseline, and checks the real source hashes.
The complete A-2 gate remains required after all its feature items are complete.
