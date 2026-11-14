# ORCHIS

**Continuous security validation for Red and Blue Teams.** Self-hosted, with a Go
backend, a React frontend, and PDF/JSON reports.

ORCHIS schedules authorised security assessments against your assets and watches
your endpoints from the inside, then puts everything into one queue: what was
found, how severe it is, and whether the fix actually worked.

![ORCHIS findings view](docs/ui-audit/shots/after/findings--dark--en--1440.jpg)

<sub>Screenshot uses synthetic demo data.</sub>

## What it does

| Side | What runs | What you get |
|---|---|---|
| **Red** (orchis-redteam-agent) | Nmap and Nuclei as preparation, then web testing through an isolated worker | Validated findings with evidence and remediation advice |
| **Blue** (DefenseBot) | A lightweight endpoint agent collects approved osquery data about software, accounts, and network | CVE matches for installed software, plus hardening findings |
| **Monitoring** | The same agent reports about once a minute | Detections for new listening ports, new local accounts, suspicious outbound connections, and agent tampering |

The main flow is:

**Organisation → Assets and agents → Scan → Findings → Report → Fix and retest**

A full assessment picks the engine per asset: an approved URL runs the Red Team
agent, an endpoint agent runs DefenseBot, and an asset with both runs both.

Things worth knowing before you rely on a result:

- Testing requires explicit authorisation per asset, plus a valid scan window.
- A failed or partial scan is recorded as a limitation. An empty result does not
  prove an asset is safe.
- Software that cannot be mapped to vulnerability data is counted and shown as
  "not mapped". It is not silently skipped.
- Detections that are no longer observed are marked stale automatically. They are
  never deleted.

## Quick start (development)

You need Go (see `go.mod`), Node.js (see `.nvmrc`), Docker with Compose v2, and
Python 3.12+.

```bash
npm --prefix web ci
make dev-up                     # PostgreSQL on 127.0.0.1:15432
make build                      # frontend + server binary
./bin/server migrate
ORCHIS_WEB_DIR=$PWD/web/dist ORCHIS_ADMIN_EMAIL=admin@orchis.local ./bin/server serve
```

If `ORCHIS_ADMIN_PASSWORD` is empty, the first admin password is generated and
printed once in the log. See [development and tests](docs/DEVELOPMENT.md) for
scanners, the endpoint agent, and the full test suite.

## Production deployment

Production runs as a Docker Compose stack on Linux amd64 (PostgreSQL, RabbitMQ,
Elasticsearch, object storage, and Caddy for HTTPS). Secrets and state live
outside the checkout.

```bash
make hackbot-check                 # validate engine sources and pins, no Docker
sudo make prod-init                # once per new state; secrets in /var/lib/orchis
sudo python3 deploy/prod/ops.py env > .env
sudo make prod-build
sudo make prod-up
sudo make prod-probe
```

The Red Team worker needs two private engine checkouts pinned in
[`engine-lock.json`](deploy/hackbot/engine-lock.json). Prepare them first by
following the [worker build guide](deploy/hackbot/README.md#build-and-configure).

For an existing installation, keep the same `ORCHIS_STATE` and follow the
[upgrade procedure](docs/OPERASI.md); do not run init again. The first login is
in `initial-admin.json` inside the private state directory. HTTPS, agent
releases, and installer settings are covered in [deployment](docs/DEPLOY.md).
Run `make help` to list every command.

## Endpoint agent

Install the agent from **Assets → Agents** in the console. It enrols with a
single-use token, connects out to the platform only, and runs read-only checks.

| Platform | Endpoint collection |
|---|---|
| Linux x86_64 | Supported. Debian and Ubuntu packages are matched to CVEs. |
| macOS x86_64 / arm64 | Supported |
| Windows x86_64 / arm64 | Supported |

RPM-based distributions are not matched to vulnerabilities yet, and coverage of
installed applications differs per operating system. See
[DefenseBot](docs/DEFENSEBOT.md) for the details.

## Repository layout

| Path | Contents |
|---|---|
| `cmd/` | Entry points: server, agent, injector, and release tools |
| `internal/` | Domain, services, API, workers, and Go tests |
| `web/` | React application and frontend tests |
| `migrations/` | PostgreSQL migrations, run before a new server version |
| `deploy/` | Dockerfiles, development and production configuration, engine lock, CI |
| `e2e/` | Fixtures, browser tests, and end-to-end checks |
| `docs/` | Product, API, operations, and architecture guides |
| `docker-compose.yaml` | Production stack; non-secret settings come from `.env` |

Each Node component (`web/`, `e2e/`, `deploy/report/`) has its own lockfile, so
run npm with `--prefix` and not from the repository root.

## Documentation

Most guides are written in Indonesian.

- [Documentation index](docs/README.md)
- [Product flow and asset readiness](docs/FLOW.md), [user roles](docs/ROLES.md), [user guide](docs/USER-GUIDE.md)
- [Scheduled audits](docs/AUDIT-SCHEDULES.md)
- [API and OpenAPI](docs/API.md), [report schema](docs/REPORT-SCHEMA.md)
- [Operations, upgrade, backup and restore](docs/OPERASI.md)
- [Architecture](docs/ARSITEKTUR.md), [historical feature compatibility](docs/FEATURE-COMPATIBILITY.md)
- [Changelog](CHANGELOG.md)

## Limitations

- Nmap and Nuclei run from the server network, so reachability results reflect
  where the scanner sits.
- The Red Team agent needs model authentication configured in the persistent
  worker home.
- A new asset or origin needs its own approval before it can be tested.
- Chains, single tests, AI, and MITRE are advanced features and are hidden by
  default (`ORCHIS_ADVANCED_FEATURES=true`).

## Use it responsibly

Only test systems you own or are explicitly authorised to test. ORCHIS enforces
per-asset authorisation, but responsibility for scope stays with you.

## License

[Apache-2.0](LICENSE). Third-party components are listed in [NOTICE](NOTICE).
