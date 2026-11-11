# Post-scan recommendations

After a new scan reaches a terminal state and its report snapshot is saved,
Orchis records one recommendation job in the same transaction. The dedicated
`recommendations` service starts a fresh OpenCode process to review recorded
vulnerability findings. It does not consume pentest jobs.

The scan detail page shows queue progress, remediation priorities, reasons and
verification steps. Recommendations remain separate from scan evidence and
severity. They are generated for human review and never apply changes or start
additional checks. Failed or stopped scans and scans without vulnerability
findings do not make model calls. Incomplete scans retain a coverage warning.
Existing historical scans are not backfilled.

## Input and execution

The review includes up to 30 findings, ordered by known exploitation and severity.
Its input contains redacted titles, finding IDs, severity, language, coverage and
a small set of structured remediation context. Asset names, target addresses,
raw requests/responses, full reports and arbitrary evidence fields are excluded.
The UI discloses omitted findings, which remain available in the scan results.

OpenCode uses the worker's existing `opencode-go` credential and the model
`opencode-go/deepseek-v4.1-flash`. Each process has a temporary private home and
workspace. OpenCode V2 permissions deny all tool actions, including shell,
network tools, file access and subagents. This is a review of supplied data.
The process has a six-minute deadline and bounded output. Temporary data is
removed when it finishes. The main pentest worker is not restarted or reused.

The server accepts only structured recommendations for the supplied finding IDs.
Every selected finding must appear once; unknown IDs, duplicate IDs, missing
items, extra fields and unsupported priorities fail validation. Output renders
as escaped text. AI priorities do not change observed finding severity or triage.

## Queue and access

Migration `00041_scan_recommendations.sql` adds a tenant-isolated queue with a
unique scan ID. Claim tokens are stored as hashes. Only the enabled red-team
worker's bearer credential can claim or complete work. Cookie authentication and
other scanner tokens cannot claim reviews. Reading advice requires assessment,
finding and report read permissions.

One model call runs per tenant at a time. Its lease lasts eight minutes. A worker
crash or an expired lease fails the job instead of repeating a possibly billed
call. Result delivery can retry without repeating the model request. Deleting
the report or scan deletes its advice. Expired scan results cannot be claimed,
read or completed. Scan evidence remains available if a review fails.

## Deployment and verification

Build the current `hackbot` image, which includes the review-only injector mode,
then start the server and `recommendations` service after migrations. The review
service mounts the existing OpenCode home read-only and uses a private tmpfs.

```bash
make prod-build
make prod-up
# Focused checks; PostgreSQL test service must be available.
go test ./internal/recommendations ./internal/injector
go test ./internal/server -run 'TestRecommendationRoutes|TestHackbotScanLifecycle'
```

The explicit `TestOpenCodeReviewLive` smoke test requires
`ORCHIS_LIVE_REVIEW=1` and the installed OpenCode command and worker credential.
It sends one synthetic finding to the configured model, not a live target.
