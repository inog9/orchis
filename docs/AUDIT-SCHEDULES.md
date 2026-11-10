# Scheduled security audits

Schedules are available at `/simulations/schedules`, from Scans, and from each organisation. They run the existing scan pipeline and use its reports and in-app inbox. No schedule is enabled by deploying the feature; users choose and save their own scope.

## User behaviour

- Daily, weekly (ISO weekdays), or monthly, using an explicit IANA timezone and local time. The server previews the next three occurrences and warns about current scan controls.
- Month days 29–31 clamp to the final day of shorter months. DST gaps skip the occurrence; repeated wall times run once at the first occurrence.
- Choose explicit authorised assets or all currently authorised assets in the organisation. Dynamic scopes include newly authorised assets. Targets are frozen per occurrence; readiness and consent are checked again by Launch and execution controls.
- The responsible user is the last person to save or resume the schedule. Current account status and permissions are reloaded on each launch. Pausing remains possible when a target has lost authorisation.
- Pause/resume, editing with optimistic version checks, and soft deletion. Resuming computes a future occurrence; it does not replay a backlog. Deleting stops future launches; existing scans and their results remain in Scans.
- Runtime limit: 15–1440 minutes (default 360). This includes queue time after the assessment is created. The scheduler stops its own overdue assessment through normal cancellation.
- Upcoming and historical times use the schedule's current timezone. Every run stores its scheduled UTC instant and selected input; settings edits affect future occurrences. Runs already claimed retain their frozen input and responsible user.

## Reliability and controls

A 30-second server worker serializes per tenant using a PostgreSQL advisory transaction lock. Claiming an occurrence, advancing its next time, and recording a missed occurrence are atomic. A run is committed before Launch and uses `schedule:<run UUID>` as its scan idempotency key. Recovery links any previously committed assessment before attempting a launch again. Schedule creation also has a UUID request key plus canonical request hash so retrying an ambiguous response cannot create another schedule.

Launch serializes against schedule pause/edit/deletion under the existing scan-control lock, reloads the responsible user's current role, and runs the existing authorisation, consent, readiness and emergency/window checks. Manual and scheduled audits cannot overlap waiting/running jobs on the same assets. Different assets can still run concurrently.

Due audits have a 15-minute start grace period. Older occurrences are recorded as missed, with one record representing the skipped backlog; the worker advances directly into the future. Policy, readiness and overlap failures are recorded as blocked and retried at the next occurrence. Infrastructure errors retain the queued occurrence for idempotent recovery, subject to that start grace period. No burst of catch-up scans occurs after downtime.

All tables enforce tenant RLS and tenant-qualified foreign keys. Reading requires organisation, asset, assessment, simulation, finding and report read permissions; writing also requires assessment/simulation run permissions. Every mutation and completed run writes an audit event.

## Results and notifications

The scheduler consumes frozen report comparisons; it does not invent a parallel finding identity or infer resolution from failure. Summaries include new, reappeared, persistent, verified-resolved and uncompared findings, plus completed versus total checks. A comparable new finding with a matching historical asset/finding identity in the same findings epoch is classified as reappeared. First observations remain new.

- Critical/high change alerts link to the scan. Persistent findings do not generate another change alert. First-run critical/high observations are included.
- Coverage-gap alerts link to the schedule and deduplicate one unresolved episode. A complete successful run releases the episode key, preserving the old notification and allowing later failures to notify again.
- An optional weekly inbox digest reports executions, coverage gaps and critical/high findings flagged over the preceding seven days. It does not claim to count unique outstanding vulnerabilities.
- Notifications contain summary counts and authenticated app links, with no finding evidence. Existing report severity/priority and comparison views remain the review destination. Notification titles and the interface support English and Indonesian; stored diagnostic reasons retain server wording.

## API

- `GET/POST /api/v1/audit-schedules` (`org`, `cursor` for listing; 50 items/page).
- `POST /api/v1/audit-schedules/preview?org=<optional UUID>` accepts a rule and returns three UTC instants plus optional restriction flags.
- `GET/PUT/DELETE /api/v1/audit-schedules/{id}`. Update includes `version`; delete accepts `{version}`.
- `POST /api/v1/audit-schedules/{id}/pause` accepts `{paused, version}`.
- `GET /api/v1/audit-schedules/{id}/runs?cursor=...` (30 items/page).

Creation accepts `{request_key, name, organization_id, config, paused}`. Config contains `kind` (`web`, `defense`, `full`), `rule` (`frequency`, `timezone`, `time`, `weekday`, `month_day`), `all_assets`, `asset_ids`, `language`, `max_runtime_minutes`, `notify_changes`, `notify_gaps`, `weekly_digest`. The per-tenant cap is 1000 live schedules; each scan retains the existing host limit.

## Deployment and rollback

Migration 40 only adds two tables and indexes. Build the server from the production-aligned backend source and the frontend from this branch. The release runtime preserves the existing worker binaries, CA certificates, MITRE data and licenses, replacing the server binary and entire public web directory. Run `server migrate` using the existing migrate service configuration before recreating only the server service. Keep an immutable tag for the previous running image.

Rollback retags that previous image and recreates only server. Keep the additive schema in place; do not roll back the database or remove schedule history. While the old server runs, scheduled audits are inactive. On upgrading again, missed occurrences follow the normal grace/backlog rules.
