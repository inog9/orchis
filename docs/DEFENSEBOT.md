# DefenseBot

DefenseBot maps the software installed on an enrolled endpoint to known vulnerabilities and lists a short set of
hardening gaps. It is the host-side counterpart of the web pentest (Nmap, Nuclei, then the OpenCode/Hackbot worker). A person decides when it
runs: press **Run DefenseBot** on an asset that has an agent.

## Flow

1. The agent (Linux or macOS) runs fixed, read-only osquery checks that an administrator approved in the current
   consent document. Nothing is sent from the server to the agent except the names of approved checks.
2. **Run** queues a fresh collection and creates a scan with the `defensebot` tool. The job waits for the collection, then
   the DefenseBot worker maps the stored data. No host name, address, account or file content leaves Orchis.
3. Findings (type `vulnerability`, evidence in `details`) and a coverage summary are stored. The frozen report gets a
   DefenseBot chapter: fleet overview, "fix first", most affected software, one section per host, hardening gaps,
   coverage notes and the data timestamps.

## Data and sources

| Data | Source | Method value |
|---|---|---|
| Ubuntu and Debian packages (by source package) | OSV (osv.dev), which aggregates the distribution security trackers | `osv-distro` |
| Global npm and Python packages | OSV (GitHub Advisory Database) | `osv-exact` |
| Cursor and Visual Studio Code (reviewed dictionary) | NVD API by CPE | `nvd-cpe` |
| Exploited in the wild | CISA Known Exploited Vulnerabilities | field `kev` |
| Exploit probability | FIRST EPSS | field `epss` |

Severity policy: advisory severity label first (distribution priority or GitHub rating), then the CVSS v3 base score
computed from the advisory vector, then the distribution urgency. An advisory without any rating is treated as medium and
marked `unrated`. KEV and EPSS change the order of "fix first", never the severity.

Fix status per CVE: `fix_available`, `no_fix_yet`, `fix_requires_subscription`, `fix_pending_reboot` (a newer kernel is
installed but not running). Advisories that are negligible and have no fix are counted, not listed.

### "Seen by scanner"

A CVE is tagged `exposed` when its software listens on all interfaces **and** a finished Nmap scan of the same asset from
the last 7 days saw that port open. Without a recent Nmap result the best possible tag is `listening`. Nmap runs from the
Orchis network, so this means "seen from the scanner", not "reachable from the internet". Ports published by a container
proxy are not attributed to a package.

## Coverage and honesty

A run is `incomplete` (job status `incomplete`, banner in the report) when the vulnerability service could not be reached,
required data is missing or truncated, the distribution is not supported yet (RPM-based distributions), or applications
could not be mapped. A missing finding never means the software is safe. Advisory text stays in English as published.

## Operations

- Service `defensebot` (same hardened image as the scanners). `ops.py up` prepares its credentials for installations that
  were created earlier (`ops.py hackbot-init` does the same explicitly) without rotating any existing secret.
- Outbound access is limited to the `scan` network. Optional free NVD key: add `"ORCHIS_NVD_API_KEY"` to the worker's
  private file `defensebot.json`; it only raises the rate limit.
- `ORCHIS_ENDPOINT_COLLECTION_SECONDS` defaults to `300`: approved checks are collected periodically every 5 minutes.
  Set 0 for manual DefenseBot runs only, or 60 to 86400 for another interval.
- Manual smoke test against the real services, never part of CI:
  `ORCHIS_LIVE_OSQUERY=/path/to/osqueryd go test -tags live -run TestLive -v ./internal/defensebot`.

## Not included yet

Windows, RPM-based distributions, container images on a host, an AI summary, and a scheduled re-match of stored data.
