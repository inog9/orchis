# Orchis user guide

Bahasa Indonesia: [PANDUAN.md](PANDUAN.md).

Orchis validates the security of networks and endpoints that your organisation
owns. The flow is:

**Organisation → Agent or Asset → Scan (Nmap/Nuclei) → Findings → Reports (PDF/JSON)**

Use Orchis only on systems you are allowed to test. Orchis refuses to scan any
target that has not been authorised.

## 1. Signing in and language

- Open the Orchis server address and sign in with your email and password. On the
  local production setup the address is `https://localhost:8443`. The initial
  certificate is **self-signed**: trust it in your browser rather than turning off
  TLS verification.
- The first admin is created when the server starts. If `ORCHIS_ADMIN_PASSWORD` is
  empty, a random password is printed once in the server log.
- An admin creates new accounts in **Settings → Users**. The new user receives a
  one-time link (valid for 30 minutes) to choose a password.
- Switch between **English** and **Indonesia** with the language selector at the top
  of the app. The choice is saved on your account. A report uses the language
  selected when the scan was started and does not change once frozen.

## 2. Roles

| Capability | Admin | Analyst | Viewer |
|---|---|---|---|
| Read findings and reports, download reports | Yes | Yes | Yes |
| View organisations, assets and agents; run scans on authorised targets | Yes | Yes | No |
| Create reports and retry PDFs | Yes | Yes | No |
| Authorise targets, endpoint consent, network ranges | Yes | No | No |
| Manage agents, users, settings, retention, emergency stop | Yes | No | No |

Viewers only see the Findings and Reports menus. Details: [ROLES.md](ROLES.md)
(Indonesian).

## 3. Creating an organisation and installing an agent

1. Open **Organisations** and create one. Its page has the tabs Overview,
   Agents/Endpoints, Assets, Scans, Findings and Reports.
2. In the organisation choose **Install agent**. Pick the OS (Linux, macOS or
   Windows) and the mode (service or user). Orchis shows a one-time command that
   downloads the agent, checks its SHA-256, enrols it and installs it. Run it on
   the target machine (as Administrator on Windows).
3. The agent appears under **Agents/Endpoints** with an online status.

> **Status:** the command shown in the console is still the older install
> command. The console has not been switched to `install.sh` yet (the frontend
> change is pending). The server already serves `install.sh` (below); until the
> console is switched, fill in its arguments by hand from the values the console
> shows.

### Installing an agent with `install.sh` (Linux and macOS)

The server serves the installer at `https://<server>/install.sh`. The script contains
no secrets; the token, hash and release key are passed as arguments from the console.

```bash
curl -fsSL https://<server>/install.sh -o install.sh   # the system CA must trust the server
sh install.sh --server https://<server> --token <TOKEN> \
  --sha256 <BINARY_SHA256> --release-key <RELEASE_KEY_BASE64> \
  --ca-fingerprint <CA_SHA256>          # or: --pin sha256//<BASE64>
```

- If the server uses an internal CA (not yet trusted by the system), a plain `curl` of the script fails. Fetch `ca.pem` from the server, compare its fingerprint over another channel, then use `curl --cacert ca.pem ...`; do not use `-k`.
- The binary is installed only if its **SHA-256 matches** and its **Ed25519 signature**
  is valid for that version, platform and hash (needs `curl` and OpenSSL 3 or newer).
- TLS verification is **never turned off** (no `curl -k`). The server certificate is
  trusted in one of two ways:
  - `--ca-fingerprint`: the script fetches `/ca.pem` without trusting it, accepts it
    only if the SHA-256 of its DER matches the fingerprint from the console, then saves
    it as `ca.pem` in the agent folder. This fits the internal CA of the Docker Compose
    install; compare it with `python3 deploy/prod/ops.py ca-fingerprint`.
  - `--pin sha256//<BASE64>`: the server certificate must have the same SPKI as the
    pin (for a self-signed server).
- Plain `http://` is accepted for a loopback server only.
- Other options: `--name` (several agents on one machine), `--mode service|user`, `--dir`.

**Honest limitation:** in `--pin` mode the agent still trusts only the system CA, so
**automatic agent updates against a self-signed server with a pin do not work**. For
automatic updates on a server with an internal CA, use `--ca-fingerprint`. Agent
support for `--ca-file` reaches machines with the next agent release on the 1.3.x line.

**macOS:** the agent runs, but osquery endpoint data is not supported on macOS yet
(Linux amd64 only). **Windows:** `install.sh` does not run on Windows; use the PowerShell
command from the console (osquery on Windows is not supported yet either).

### Uninstalling an agent

```bash
sh install.sh --uninstall            # stop the service, remove the binary, keep config and data
sh install.sh --uninstall --purge    # also delete the agent's configuration and data
```

Dangerous directories (`/`, `/opt`, `/usr`, `/home`, `/root`) are refused. Also revoke
the agent in the console if it will not return.

Enrolling an agent does **not** allow scanning. Several agents on one machine
need different instance names (`--name`).

## 4. Assets and authorisation

- Add assets one at a time on the **Assets** tab, or add a host found by a network
  scan with one click.
- An admin fills in the target address and then **authorises testing** for each
  asset, or for a network range that belongs to an agent. An asset without
  authorisation cannot be scanned.
- A network range from an agent can only be scanned when an admin approves it, the
  endpoint has a valid consent, and the agent reported its interfaces and routes
  within the last 24 hours.

## 5. Running a scan

1. Press **Scan** (from the organisation, or under **Simulations**).
2. Choose the organisation, then the targets: one or more authorised assets and/or
   ranges from one agent.
3. Tick **Nmap** and/or **Nuclei** and pick the **Quick** or **Standard** preset.
4. Press the run button. Targets and tools run in parallel; one failing target does
   not stop the others.

Good to know:

- Scans run from the **Orchis server**, not from the agent. A network range must be
  reachable from the server. An unreachable range is recorded as **failed**; that
  does not mean the host is safe.
- Default limits: 256 addresses per scan and 4 active jobs.
- Results appear per target: live hosts, open ports and services (Nmap), and
  findings with severity, template and evidence (Nuclei).

### Scan window and emergency stop (Admin)

- **Scan window and emergency stop** on an organisation limits scans to chosen days
  and hours (IANA timezone). With no window, scans are allowed at any time.
- **Stop all scans** (also in Settings, for the whole system) cancels waiting and
  running scans and holds new ones. Stopped scans are marked canceled, never safe.
  **Resume** only allows new scans; it does not restart interrupted ones.
  Details: [SCAN-CONTROLS.md](SCAN-CONTROLS.md) (English).

## 6. Findings and scan comparison

- **Findings** and **Assets** can be filtered by organisation.
- Each finding is compared with the previous scan of the same check: **New**,
  **Still present** or **Resolved**. If the earlier or current check failed, was
  skipped or was stopped, the result is **Not compared**; a failed scan never marks
  a finding as resolved. Details: [SCAN-COMPARISON.md](SCAN-COMPARISON.md).

## 7. Reports

- A report is a frozen snapshot per organisation, downloaded as **PDF** or **JSON**
  (schema: [REPORT-SCHEMA.md](REPORT-SCHEMA.md)).
- If the PDF fails, press **Retry PDF generation**; the scan is not repeated. The
  JSON stays available.

## 8. Endpoint data (osquery)

osquery runs inside the agent and collects data only after approval. Open the
organisation → **Agents/Endpoints** → choose an endpoint → **Configure endpoint
collection**. The queries supported by the endpoint's OS are suggested, but the
consent box stays empty until an admin approves it. Collection runs periodically
(15 minutes by default). An admin can delete the latest dataset from the asset
detail. The consent text is in [ENDPOINT-CONSENT.md](ENDPOINT-CONSENT.md).

**Limitation:** endpoint data is only proven on **Linux amd64**. macOS and Windows
show as *unsupported* and have not been tested on real devices.

## 9. Notifications

The bell shows the number of unread notifications; the **Notifications** page keeps
the history. Events include scans finished or stopped, PDF ready or failed, agents
offline or revoked, agent update failures, and authorisation or consent requests
waiting for an admin. There is no email or webhook. An admin can change the
offline threshold on the Notifications page (15 minutes by default).
Details: [NOTIFICATIONS.md](NOTIFICATIONS.md) (Indonesian).

## 10. Managing agents (Admin)

Open **Assets → Agents → Manage**. An admin can:

- see the agent version, the reported osquery version, OS, last contact and update status;
- choose **Automatic for all agents** or **Manual per agent** rollout, then **Update now**;
- **Pause/Resume** an agent (stops new tasks only) and **Collect data now**;
- revoke an agent or uninstall it remotely, and read safe diagnostics.

An agent only installs a valid signed release and never downgrades.
Details: [AGENT-CONTROLS.md](AGENT-CONTROLS.md) (Indonesian).

## 11. Data retention and deleting an organisation (Admin)

- **Settings → Data retention** sets how long data is kept: findings (180 days),
  scan results (90), reports (180), endpoint datasets (30) and audit (365). Older
  data is removed automatically. Check your backups before shortening a period.
- **Review deletion** on an organisation's Overview shows the impact of deleting the
  organisation before you confirm. Details: [RETENTION.md](RETENTION.md) (Indonesian).

## 12. Troubleshooting

| Symptom | Cause and action |
|---|---|
| "This asset is not authorized for testing" | An admin has not authorised the asset or its address. |
| A network range failed | The range is not reachable from the Orchis server. Check server routing and firewall. |
| Scan refused: window closed or on hold | Outside the scan window, or the emergency stop is active. An admin can change the window or press Resume. |
| Agent shows offline | No heartbeat past the threshold. Check the agent service and its connection to the server. |
| PDF failed | Press **Retry PDF generation**; check the renderer and MinIO. JSON stays available. |
| UI looks unchanged after an upgrade | Hard-refresh the browser (Ctrl+Shift+R). |

Operations (upgrade, backup, restore) are in [OPERASI.md](OPERASI.md) (Indonesian).
