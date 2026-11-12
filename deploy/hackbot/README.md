# Nmap + Nuclei → OpenCode (orchis-redteam-agent) web pentest

Orchis uses UI v3 and the existing queue and PDF report service. The orchis-redteam-agent
worker runs OpenCode with the pinned upstream pentest tools.
Users select approved HTTP(S) assets and click **Run pentest**. Per selected asset:

1. Nmap service discovery on the approved URL’s port and Nuclei vulnerability scanning run first, in parallel.
2. orchis-redteam-agent starts only after both scanner jobs reach a terminal result.
3. The worker snapshots the preliminary findings, targets, parameters, execution
   IDs and status/errors. orchis-redteam-agent receives those files in its own engagement
   workspace before the first prompt.
4. orchis-redteam-agent independently tests relevant leads, records verified findings, and
   returns its results. Findings, coverage, a summary of at most 300 words, and
   the PDF report are saved and displayed automatically inside Orchis.

The OpenCode handoff location is:

```
<data>/<hackbot-execution-id>/
  inputs/scanners/
    manifest.json     # schema, filenames, SHA-256 digests and byte sizes
    nmap.json         # preliminary Nmap findings and execution metadata
    nuclei.json       # preliminary Nuclei findings and execution metadata
  private/
```

A private staging copy is also kept under the execution's `scanner-inputs/`
folder. Imports accept only these fixed filenames and verify hashes and bounds;
symlinks or changed existing evidence are refused. The files contain findings
snapshots, not raw XML/model transcripts. Raw scanner output remains in Orchis's
existing private output storage.

Scanner observations are untrusted leads, not automatically validated orchis-redteam-agent
findings. If a preliminary scanner fails, orchis-redteam-agent receives its failure metadata
and the remaining findings. The overall report remains incomplete even if orchis-redteam-agent
finishes. Cancellation or revoked authorization prevents further stages.

## Build and configure

The build needs **both private Git checkouts**, not extracted source archives:

```
orchis/
orchis-hackbot-lite/
orchis-opencode-hackbot/
```

Clone with an account that can read the repositories named in
[`engine-lock.json`](engine-lock.json), then check out the exact pinned revisions.
From the Orchis root:

```sh
git clone https://github.com/r1pu5u/orchis-hackbot-lite.git ../orchis-hackbot-lite
git clone https://github.com/r1pu5u/orchis-opencode-hackbot.git ../orchis-opencode-hackbot
python3 - <<'PINS'
import json, subprocess
from pathlib import Path
lock = json.loads(Path('deploy/hackbot/engine-lock.json').read_text())
for directory, pin in [('../orchis-hackbot-lite', lock), ('../orchis-opencode-hackbot', lock['opencode'])]:
    subprocess.run(['git', '-C', directory, 'checkout', '--detach', pin['revision']], check=True)
PINS
make hackbot-check
```

Existing checkouts must contain the pinned commits; fetch them if necessary before
checking out. Review local changes before switching revisions.
`ORCHIS_HACKBOT_SOURCE` and `ORCHIS_OPENCODE_SOURCE` select alternate paths.
Export both when building with custom locations; preserve them when invoking the
build under another operator account.

`make hackbot-check` checks revisions and integration file hashes without Docker.
`make hackbot-engines` validates both sources before building the tool image and
then the OpenCode image. `make hackbot-image` also builds the worker injector from
this Orchis checkout. `make prod-build` runs the same source preflight **before any
production image build**, then builds the complete stack. GitHub credentials are
used only for cloning, never passed into Docker layers.

If a checkout is missing, the preflight prints its path, repository and required
revision. An image already present in Docker is not a substitute for the source
checkouts needed by a reproducible build. Do not bypass the revision/hash checks
or create artificial Git metadata around an archive.

Fresh `ops.py init` generates credentials for all four workers. Existing setups
run `ops.py hackbot-init --state /path/to/existing/state` before provisioning the
updated images. It preserves existing valid scanner tokens and adds missing
worker files. Apply the pending migrations and use the normal migrate/provision services.
Nmap/Nuclei/DefenseBot workers run as UID 10001; the orchis-redteam-agent worker uses UID 1001.

The pinned OpenCode worker reads the `opencode-go` credential from the private
`/home/pentest/.pi/agent/auth.json` store in the persistent `hackbot-home` volume.
This legacy path is retained for compatibility; OpenCode runs the engagement,
not Pi. Provision the credential securely once; Orchis does not supply model
credentials. The pinned worker uses `opencode-go/deepseek-v4.1-flash` and bounds
its model run to four hours after stack setup. Source and configuration details are in the pinned
OpenCode repository.

Missing authorized test accounts produce coverage limitations. orchis-redteam-agent receives
permission for the exact selected origin; discovered origins require separate
approval. Its full tools are not a network sandbox. No user Markdown viewer or
download is required.

## Operations

- A web pentest queues three jobs per web asset; it does not create a Docker container.
  One active orchis-redteam-agent engagement runs per worker container. Scale workers for
  parallel testing. Existing scanner concurrency remains bounded.
- Execution leases, scan controls and live authorization apply throughout the
  pipeline. Orchis scanner/simulation duration limits do not terminate orchis-redteam-agent;
  the pinned OpenCode worker applies its own four-hour model limit after stack setup.
- Claimed orchis-redteam-agent jobs are not automatically retried. Reconcile ambiguous target
  state before operator-led continuation.
- Reports retain individual Nmap, Nuclei and orchis-redteam-agent method provenance. Partial
  checks cannot establish resolution for their missing coverage.
- Internal validation is JSON through the audited admin endpoint
  `GET /api/v1/scan-jobs/{id}/validation`. Raw identity/model evidence stays private.
- Workers prune private execution directories hourly only after authenticated
  retention checks confirm deletion of the owning execution. API errors retain
  evidence. Engine data/home are included in encrypted backup format 2.
- Restore old backup formats with their matching release, then upgrade.

## Validation

`go test ./internal/server -run ^Testorchis-redteam-agent` covers stage ordering, immutable
scanner handoff, scanner failure, result ingestion, PDF access, authorization
revocation and lease expiry. Worker tests cover private files, changed handoffs
and claim payload size. OpenCode engine tests cover scanner-input imports, hashes and private-evidence
result validation. Run `npm test` in that companion checkout. Build preflight
and orchestration tests run without Docker via `make build-test`.

The explicit production smoke profile uses real local Nmap/Nuclei stages and a
synthetic orchis-redteam-agent result transport fixture, with no paid model. It never claims
that the synthetic orchis-redteam-agent phase performed a real pentest. Older E2E fixtures
that submit partial tool selections need migration to the three-stage scan API;
advanced scanner contracts and historical APIs remain available.

## Verify a deployment

Use `make prod-up` with the existing `ORCHIS_STATE` to wait for core health and
worker heartbeats. `GET /api/healthz` confirms API health; it does not prove that a
model-backed pentest succeeded. Inspect the orchis-redteam-agent container logs and its
integration heartbeat after replacement, and compare the running container image
ID with the image just built. A real engagement is a separate authorized test.

## Compatibility identifiers

The product name is `orchis-redteam-agent`. The API tool `hackbot`, contract
`hackbot.pentest`, `ORCHIS_HACKBOT_*` settings, and deployment service names remain
stable so existing jobs and workers keep working.
