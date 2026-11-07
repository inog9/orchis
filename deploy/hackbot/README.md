# Nmap + Nuclei → Hackbot web pentest

Orchis uses its current UI v2 and the existing queue and PDF report service.
Users select approved HTTP(S) assets and click **Run pentest**. Per selected asset:

1. Nmap service discovery on the approved URL’s port and Nuclei vulnerability scanning run first, in parallel.
2. Hackbot starts only after both scanner jobs reach a terminal result.
3. The worker snapshots the preliminary findings, targets, parameters, execution
   IDs and status/errors. Hackbot receives those files in its own engagement
   workspace before the first prompt.
4. Hackbot independently tests relevant leads, records verified findings, and
   returns its results. Findings, coverage, a summary of at most 300 words, and
   the PDF report are saved and displayed automatically inside Orchis.

The exact handoff location, next to Hackbot's reports and recon folders, is:

```
<data>/<hackbot-execution-id>/.pentest-harnes/sessions/<session-id>/workspace/
  inputs/scanners/
    manifest.json     # version, filenames, SHA-256 digests and byte sizes
    nmap.json         # preliminary Nmap findings and execution metadata
    nuclei.json       # preliminary Nuclei findings and execution metadata
  recon/
  reports/
  private/
```

A private staging copy is also kept under the execution's `scanner-inputs/`
folder. Imports accept only these fixed filenames and verify hashes and bounds;
symlinks or changed existing evidence are refused. The files contain findings
snapshots, not raw XML/model transcripts. Raw scanner output remains in Orchis's
existing private output storage.

Scanner observations are untrusted leads, not automatically validated Hackbot
findings. If a preliminary scanner fails, Hackbot receives its failure metadata
and the remaining findings. The overall report remains incomplete even if Hackbot
finishes. Cancellation or revoked authorization prevents further stages.

## Build and configure

Keep compatible source checkouts next to one another:

```
orchis/
orchis-hackbot-lite/
```

Check out the companion engine commit pinned in `engine-lock.json`.
`ops.py build` builds the server, report renderer, scanner image, pinned local
engine and Hackbot worker. `ORCHIS_HACKBOT_SOURCE` selects another engine checkout.
GitHub credentials are needed only for cloning, not inside image layers.

Fresh `ops.py init` generates credentials for all three workers. Existing setups
run `ops.py hackbot-init --state /path/to/existing/state` before provisioning the
updated images. It preserves existing valid scanner tokens and adds missing
worker files. Apply migration 36 and use the normal migrate/provision services.
Nmap/Nuclei workers run as UID 10001; the Hackbot worker uses UID 1001.

Configure Pi model/provider authentication once in the persistent engine home:

```sh
# Use the same Compose environment and state as the installation.
docker compose run --rm --no-deps hackbot pi
```

Provider credentials are not supplied by Orchis. Missing authorized test accounts
produce coverage limitations. Hackbot receives permission for the exact selected
origin; discovered origins require separate approval. Its full tools are not a
network sandbox. No user Markdown viewer or download is required.

## Operations

- A Run click queues three jobs per asset; it does not create a Docker container.
  One active Hackbot engagement runs per worker container. Scale workers for
  parallel testing. Existing scanner concurrency remains bounded.
- Execution leases, scan controls and live authorization apply throughout the
  pipeline. Hackbot is exempt from scanner and simulation duration limits.
- Claimed Hackbot jobs are not automatically retried. Reconcile ambiguous target
  state before operator-led continuation.
- Reports retain individual Nmap, Nuclei and Hackbot method provenance. Partial
  checks cannot establish resolution for their missing coverage.
- Internal validation is JSON through the audited admin endpoint
  `GET /api/v1/scan-jobs/{id}/validation`. Raw identity/model evidence stays private.
- Workers prune private execution directories hourly only after authenticated
  retention checks confirm deletion of the owning execution. API errors retain
  evidence. Engine data/home are included in encrypted backup format 2.
- Restore old backup formats with their matching release, then upgrade.

## Validation

`go test ./internal/server -run ^TestHackbot` covers stage ordering, immutable
scanner handoff, scanner failure, result ingestion, PDF access, authorization
revocation and lease expiry. Worker tests cover private files, changed handoffs
and claim payload size. Engine tests exercise imports into the actual Pi workspace
before prompting, hashes, unsafe paths and partial delivery.

The explicit production smoke profile uses real local Nmap/Nuclei stages and a
synthetic Hackbot result transport fixture, with no paid model. It never claims
that the synthetic Hackbot phase performed a real pentest. Older E2E fixtures
that submit partial tool selections need migration to the three-stage scan API;
advanced scanner contracts and historical APIs remain available.
