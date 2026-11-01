Endpoint data collection consent
Version 2026-11-01.1

Purpose and timing
Collect read-only endpoint inventory and security posture for this organisation. Collection runs only when an authorised user starts a named check or a scenario containing it. Enrollment does not grant permission to test an asset. Low-sensitivity baseline checks are enabled by default; other checks and every official pack require an administrator's approval. There is no collection schedule in this release.

Data read
The selected check or pack lists the exact tables and columns before approval. Depending on that selection, data can include operating system, kernel, hardware, hostname, installed packages and versions, local account and group names, listening ports and process names, and firewall or hardening settings. Browser history/extensions, private SSH keys, process arguments/environment and user-file contents are excluded. Pack queries not approved for this release stay skipped. Queries needing more privileges are skipped with an explicit reason when the agent lacks them.

Storage, reports and access
The server treats agent data as untrusted, keeps only known columns, redacts credential-shaped text, limits cells and rows, and bounds each sanitized dataset to 1 MiB. Free-form output and agent-supplied findings/logs are discarded for osquery. The latest successful dataset is stored separately from findings; a failed attempt preserves that earlier dataset and records the failure. Users with asset-read permission can see endpoint rows and mapped summaries. Findings and report exports follow their own permissions. Administrators and authorised infrastructure operators can also access their managed storage and backups. Only approved mapping rules produce report findings and posture summaries. This approval does not enable an external AI provider.

Retention
The current release keeps the latest successful dataset until a successful replacement or explicit deletion. It has no automatic age-based expiry and no collection history yet. Findings, frozen report snapshots, audit events and encrypted backups are separate records and are not erased by deleting the latest endpoint dataset. The organisation must apply its own retention policy to those records and backup copies. Do not approve collection if this retention model does not meet that policy.

Stop and delete
Turn off a named check or pack on the organisation's Endpoint data tab to stop new collections of those named queries. In-flight results are discarded after approval changes. To stop every action on an endpoint, revoke its testing authorisation or revoke the agent; legacy basic agent actions have separate testing permission. An administrator with asset-write and settings-management permission can use Delete collected data on the asset's Endpoint data tab. That deletes the latest datasets for the asset and invalidates in-flight results, without deleting findings, audit events, reports or backup copies. Deletion does not disable queued or future manually requested checks; turn them off or revoke testing permission before deletion to prevent recollection. Uninstall the agent if it should no longer contact the server.

Approval record
Approving records the administrator's identity, server time, document version and SHA-256, along with the selected query/pack, tables and frozen pack version. A changed document requires renewed approval for checks that need explicit consent. Revocation is recorded separately in the audit log. This is an operational collection permission, not a substitute for the organisation's legal basis or endpoint-owner agreement.
