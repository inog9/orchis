# Docker pentest stack

The image runs Pi, the harness, a Gori capture proxy, headless Chromium with the
Interceptor browser extension, and the `r1pu5u/jxscout` fork. It needs no desktop
window, Burp installation, model credentials during build, or exposed service
ports. Use one active engagement per container.

## Build and start

```sh
docker compose build
docker compose run --rm pentest
```

The image supports Linux amd64 and arm64. The Dockerfile pins Go, Node, Bun,
Pi packages, Gori 0.9.1, Interceptor 2.1.11, and the jxscout source commit.
Gori and Interceptor release files are checked against pinned SHA-256 hashes.
Chromium and shared libraries come from the base distribution's package index.

For the first provider login, open Pi directly:

```sh
docker compose run --rm pentest pi
```

Configure your model/provider in Pi, exit, then launch the harness normally.
The `pi-home` volume preserves Pi settings and credentials. The `engagements`
volume preserves sessions, reports, traffic, and browser profiles under `/data`.
Credentials are not copied from the host or embedded in the image.

The image seeds the four runtime Pi packages into a new home. Existing settings
are preserved. Do not use `docker compose down -v` unless you intend to delete
both saved engagements and provider credentials.

```sh
docker compose run --rm pentest pentest-harnes sessions
docker compose run --rm pentest pentest-harnes --session SESSION_PREFIX
docker compose run --rm pentest pentest-harnes doctor
```

Use `docker compose run --rm --entrypoint sh pentest` for maintenance. Override
the default command with `pi` or `pentest-harnes` as shown above. No host Docker
socket or browser profile is mounted into the container.

## Submit one prompt from another service

Use the image's `pentest-job` command for an unattended job. It wraps the harness
and Pi RPC lifecycle, accepts one engagement prompt, and runs until complete,
limited, blocked on required input, cancelled, or failed. It does not start an
HTTP server. The command is installed as a symlink to
`/opt/pentest/docker/run-job.mjs`.

Provision the Pi model/provider settings and authentication in the persistent
`pi-home` volume before submitting jobs, using the provider-login command above.
An unattended job cannot answer an interactive model login. Environment-referenced
credentials in your Pi configuration are also supported; explicitly pass only the
required variables into the job container. Neither the prompt nor the image
build should contain model-provider secrets.

Send the prompt through stdin rather than interpolating it into a shell command:

```sh
docker compose run --rm -T pentest pentest-job --prompt-file - < job-prompt.txt
```

The prompt should state the authorized targets, permitted actions, accounts when
needed, and any test window or request bounds. Required missing input ends the
job with a resumable session; it does not invent authorization or credentials.
Alternatively, mount a prompt file read-only:

```sh
docker compose run --rm -T \
  -v "$PWD/job-prompt.txt:/run/job-prompt.txt:ro" \
  pentest pentest-job --prompt-file /run/job-prompt.txt
```

For example, another service can launch the job without shell interpolation.
Run this with the repository's Compose directory as its working directory:

```python
import json
import subprocess

result = subprocess.run(
    ["docker", "compose", "run", "--rm", "-T", "pentest",
     "pentest-job", "--prompt-file", "-"],
    input=engagement_prompt,  # One string supplied by your job queue.
    text=True,
    capture_output=True,
)
for line in result.stdout.splitlines():
    event = json.loads(line)
    # Store or forward the structured lifecycle event to your job system.
handle_job_exit(result.returncode)
```

Stdout is JSONL lifecycle status suitable for a submitting service. Each event
has `schema: 1`, `event`, `at`, and `jobId`; `sessionId` is included once known.
Events are `starting`, `started`, `state`, `stopping`, and one terminal event:
`complete`, `limited`, `input_required`, `failed`, or `cancelled`. State updates
contain phase/mode/revision and numeric task counts, rather than model text.
The `started` and terminal events identify the session root, workspace, and logs.

Raw prompts, model output, questions, and diagnostics remain in the session's
`private/jobs/<jobId>/rpc.jsonl` and `stderr.log`, alongside the private service
logs. `input_required` reports a stable reason such as `interactive_request` or
`paused`; obtain the actual question from the private logs through your trusted
operator flow. Do not expose those files or stderr automatically to an untrusted
job submitter. The terminal lifecycle status and exit code distinguish successful
completion from partial coverage and required input:

| Exit | Meaning |
| --- | --- |
| `0` | Complete |
| `2` | Limited delivery; inspect recorded coverage limits |
| `3` | Required input; retain the session ID and obtain the missing information |
| `1` | Failure; inspect private diagnostics before deciding whether to resume |
| `130` | Cancelled |

`pentest-job` accepts `--dir` (default `/data`), `--session ID` to resume an
existing engagement, and `--name` for a new engagement. `--pi` and `--harness`
select executable paths for controlled/test deployments. To continue after
supplying missing information, send a new instruction to the same session:

```sh
docker compose run --rm -T pentest pentest-job \
  --session SESSION_ID --prompt-file - < follow-up-prompt.txt
```

Start one container per active engagement. Separate `/data` volumes are simplest;
a shared persistent `/data` is supported through distinct UUID session directories.
Never submit concurrent jobs for the same session. Each container has private
loopback proxy services, so ports do not collide across containers. Preserve Pi
authentication/configuration separately from engagement data when allocating
job volumes. An exit code is not permission to retry a request: reconcile
interrupted work and actual target state before resuming ambiguous mutations.

## Automatic gog setup in Docker

Every container command, including `pentest-job`, runs the local gog bootstrap
first. It prepares persistent storage and imports only explicitly supplied
credential files. It does not read a host mailbox, start OAuth consent, check
Google connectivity, or fetch mail. Once an authorized account has been imported
or consented, later containers reuse it automatically from the `pi-home` volume.
Without tokens, independent pentest work continues and Gmail verification remains
blocked until the account is provisioned.

Defaults preserve an explicit operator override:

| Setting | Docker default / purpose |
| --- | --- |
| `GOG_HOME` | `/home/pentest/.gog`, persisted in `pi-home` |
| `GOG_KEYRING_BACKEND` | `file`, encrypted credentials without a desktop keychain |
| `DBUS_SESSION_BUS_ADDRESS` | `disabled:`, preventing desktop D-Bus autostart |
| `GOG_KEYRING_PASSWORD` | Reuse a generated private persistent password, or supply an explicit value |
| `GOG_KEYRING_PASSWORD_FILE` | Optional mounted password file instead of the explicit password environment value |
| `GOG_READONLY`, `GOG_GMAIL_NO_SEND`, `GOG_WRAP_UNTRUSTED` | `1`, `1`, `1` for verification-oriented defaults |
| `GOG_AUTO_JSON` | `1`, machine-readable output when gog is piped |
| `GOG_OAUTH_CLIENT_FILE` | Optional mounted Desktop OAuth client JSON to import |
| `GOG_TOKEN_FILE` | Optional mounted `gog auth tokens export` JSON to import |
| `GOG_ACCOUNT` | Explicit authorized email; required for token import and must match its email |
| `GOG_CLIENT` | Named OAuth client; defaults to `default` and must match the token export |
| `GOG_TOKEN_IMPORT_REPLACE` | Set `1` to import a changed token export over an existing token; existing tokens are otherwise preserved |

Keep the persistent home and its generated keyring password together. Changing
the password does not decrypt an existing keyring. Shared-home containers use
locked import receipts so repeated starts do not reimport unchanged files. The
bootstrap performs local configuration only; actual token validity and account
access are established when an authorized Gmail task runs.

Explicit imports pin an unset/blank `GOG_CLIENT` to `default` for the child
command, keeping it in the imported token bucket. Without imports, existing gog
account/domain routing remains available.

An invalid explicit import or unavailable keyring unlock secret stops container
startup with exit `1`. It never reports a limited pentest delivery or opens an
interactive login prompt. Supplying no import files is valid: public/offline work
can still start before Gmail is provisioned.

### A. Import an account that already has consent

On an operator-controlled machine where that exact account is already authorized
in gog, export it to a private location outside the repository:

```sh
gog auth tokens export tester@example.com --out /secure/gog/token.json
```

This file contains refresh-token secrets. Also provide the corresponding Desktop
OAuth client JSON as `/secure/gog/client.json`; tokens and client must belong to
the same OAuth configuration. Restrict access while ensuring the container's
`pentest` user (UID `1001`) can read the mounted files. Mount files read-only;
do not commit them or put their contents in a pentest prompt.

For example, save this credential-mount override outside the repository as
`/secure/gog/compose.gog.yaml` (replace the example account with the authorized
one):

```yaml
services:
  pentest:
    environment:
      GOG_ACCOUNT: tester@example.com
      GOG_OAUTH_CLIENT_FILE: /run/gog/client.json
      GOG_TOKEN_FILE: /run/gog/token.json
    volumes:
      - /secure/gog/client.json:/run/gog/client.json:ro
      - /secure/gog/token.json:/run/gog/token.json:ro
```

Submit a job with the normal persistent home and this override:

```sh
docker compose -f compose.yaml -f /secure/gog/compose.gog.yaml \
  run --rm -T pentest pentest-job --prompt-file - < job-prompt.txt
```

The entrypoint imports client credentials and the matching token idempotently
before spawning the requested command. A missing, unreadable, or mismatched
explicit import is a configuration error, not permission to choose another
account. No host gog directories are mounted implicitly.

### B. Give first consent once in an operator terminal

Create a Google Cloud **Desktop app** OAuth client, enable the Gmail API, and
obtain its JSON using the [pinned gog quickstart](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/quickstart.md).
Use the override above with only the client mount: omit `GOG_TOKEN_FILE` and
its token-file volume until a token exists. Run the split OAuth flow directly
in your own terminal, outside `pentest-job`:

```sh
docker compose -f compose.yaml -f /secure/gog/compose.gog.yaml \
  run --rm -T pentest gog auth add tester@example.com \
  --services gmail --gmail-scope readonly --remote --step 1
```

Open the printed consent URL in your own browser and authorize the exact test
account. After redirect, copy the full redirected URL even if its local callback
page cannot load. In the same operator terminal, read it without pasting it into
shell history, then complete step 2:

```sh
printf 'Paste the full redirected URL: '
IFS= read -r GOG_AUTH_REDIRECT_URL
docker compose -f compose.yaml -f /secure/gog/compose.gog.yaml \
  run --rm -T pentest gog auth add tester@example.com \
  --services gmail --gmail-scope readonly --remote --step 2 \
  --auth-url "$GOG_AUTH_REDIRECT_URL"
unset GOG_AUTH_REDIRECT_URL
```

Keep the same `pi-home` volume, `GOG_HOME`, OAuth client, and account between
steps: the first step stores a short-lived PKCE verifier there. Consent and
redirect URLs belong only in this operator flow, not model prompts, general job
logs, screenshots, or reports. After success, normal jobs reuse the persisted
account; OAuth consent itself cannot be automated away. Token revocation or
expiry may still require the operator to reauthorize that same account/client.

The pinned upstream [paths](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/paths.md),
[OAuth client](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/auth-clients.md), and
[token import](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/commands/gog-auth-tokens-import.md)
references describe the storage and command contracts used here.

## Tool inventory

The Docker build installs these tools. Versions below are build inputs, not a
claim that an image has been built or exercised successfully on your machine.

| Tool | Version/source pin | Use |
| --- | --- | --- |
| Gori | `0.9.1` | Capture, MCP replay, and exported HTTP evidence |
| Interceptor Browser | `2.1.11` | Headless Chromium extension, native host, and CLI |
| jxscout | `r1pu5u/jxscout` commit `c43dbd56139a6d332bcc1bfba502f1736b7a3185` | Scoped JS collection, chunk discovery, and analysis |
| ffuf | `2.3.0` | Bounded endpoint/WordPress enumeration |
| httpx | ProjectDiscovery `1.12.0` | HTTP probing; distinct from the Python package |
| gau | `2.2.4` | Historical URL collection before archive follow-up |
| jsluice | `v0.0.0-20240110145140-0ddfab153e06` | Offline JS URLs, secrets, syntax trees, and source/sink queries |
| npxconfuse | `cybershaykh/npxconfuse` commit `c1a56e347684a37b6189bc3f12dfa9220084e11c` | Registry-name checks; optional in the skill workflow |
| gog | `openclaw/gogcli` `0.48.0` | Authorized Gmail verification flows |
| totp-cli | `1.9.2` | Authenticator codes for the exact authorized test account |
| interactsh-client | ProjectDiscovery `1.4.1` | Authorized OOB callback collection when needed |
| Prettier / Webcrack | `3.9.10` / `2.16.0` | jxscout JavaScript formatting |

The `npxconfuse` npm name resolves to a placeholder rather than the CLI used by
the skills. The build therefore checks the source archive hash and uses that
revision's lockfile, without executing package lifecycle scripts. Go CLI builds
use pinned module versions; jsluice's Tree-sitter parser is compiled with CGO.
The bundled Tree-sitter query files do not require a separate parser CLI or
Python package.

Distribution packages provide `curl`, `jq`, ripgrep (`rg`), `unzip`, `zip`,
`xz`, `file`, `openssl`, Bash, coreutils, findutils, diffutils, procps, gawk,
`sed`, `grep`, `tar`, `gzip`, and `bzip2`. Python is used with its standard library
only; no pip dependencies are needed by the bundled helpers. The WordPress
10,000-entry and 132,211-entry lists are bundled with the skills; the preparer
creates the 122,211-entry nonoverlapping second tier. No SecLists download is
required. Endpoint wordlists are derived from the engagement's own inventory.

Account-backed tools still need explicit provisioning: gog requires an
engagement-authorized Gmail OAuth account and usable credential storage;
totp-cli requires the mapped token store/unlock mechanism or an explicitly
supplied seed through a private input channel. Textbee remains a separately
configured MCP connection to an authorized phone/account. OOB tests need an
authorized callback service; installing interactsh-client does not itself grant
authorization or guarantee service connectivity. Preserve these credentials in
private storage, not the image, prompt arguments, or reports.

Tool availability does not start a scan. The orchestrator still selects scoped,
budgeted work. In particular, bulk ffuf runs direct with proxy variables cleared;
only selected evidence replays use Gori. Optional tool failures and unavailable
accounts remain explicit coverage limits rather than successful negative tests.

## Session services

After session selection the launcher starts `docker/stack.py`. Gori captures into
`private/stack/gori.db`; Pi's generated Gori MCP entry opens the same database.
The proxy listens only on container loopback at port 8070. MCP uses stdio.
Chromium uses a session-specific profile, a native messaging host, and the
unpacked Interceptor extension. The launcher waits for confirmed extension
reachability before starting Pi.

The stack creates a private Gori CA and trusts it in Chromium's session-specific
NSS store. It does not disable certificate validation. Chromium runs as the
unprivileged `pentest` user with `--no-sandbox` because the default Docker runtime
does not provide Chromium's nested sandbox; Docker remains its process boundary.
No remote debugging port, GUI, or Xvfb fallback is enabled.

Each session stores these files below its `private/stack/` directory:

| Path | Contents |
| --- | --- |
| `gori.db`, `gori/` | Captured flows and private Gori configuration/CA |
| `capture.jsonl` | Completed capture events consumed by the bridge |
| `chromium/`, `browser-home/` | Browser profile and certificate trust |
| `interceptor/` | Session-specific Interceptor transport and logs |
| `jxscout-workspace/`, `jxscout-data/` | JavaScript collection and analysis state |
| `bridge-checkpoint.json` | Bridge delivery and deduplication checkpoint |
| `*.log`, `ready.json` | Service diagnostics and startup readiness |

These files contain sensitive traffic and session data. The container uses
private permissions and the existing engagement gitignore excludes them.
Interceptor task tab groups isolate tabs; they do not isolate account cookies.
Continue to use separate authorized account/session contracts for role testing.

## Capture and JavaScript discovery

JXScout collection starts only when the controller has an authorized HTTP(S)
origin scope, permits GET, and is running within its test window. Pausing,
completion, scope changes, and window expiry stop or restart collection. Unknown
scope leaves Gori and Chromium ready but does not start autonomous discovery.

The bridge exports completed in-scope response flows through Gori's CLI and
forwards them directly to jxscout's local `/caido-ingest` endpoint. JXScout's
discovered chunk downloads use Gori as their HTTP/HTTPS proxy. The build patches
the pinned fork's custom transport to honor an explicit `JXSCOUT_PROXY_URL` and normal TLS
trust while retaining its scope, redirect, credential, and rate-limit checks.
The bridge checkpoints delivered assets and suppresses identical repeats.

Proxy environment variables apply only to jxscout; model-provider calls and local
ingestion do not pass through Gori. Each origin is translated into a full-URL
scope pattern, preserving explicit ports. Scope limits, request budgets, and
role verification still belong to the orchestrator; the container is not an
engagement-wide network firewall.

Bridge history recovery defaults to a maximum of 100,000 captured flows and
refuses larger histories explicitly. Set `PENTEST_HARNES_HISTORY_LIMIT` to a
larger positive integer when resuming larger engagements; this trades memory
and startup time for recovery coverage. Pass it through Compose with `-e`.

## Verify and troubleshoot

```sh
docker compose run --rm pentest python3 /opt/pentest/docker/tool-smoke.py
docker compose run --rm pentest python3 /opt/pentest/docker/smoke.py
```

The build runs the tool smoke automatically. It checks CLI capabilities and uses
only local ffuf/JavaScript fixtures; it does not read mail, generate an OTP, or
register an OAST callback. Missing binaries or incompatible command flags fail
the build. `doctor` reports availability and configuration separately from auth.

The smoke test creates a local HTTPS fixture, opens it through headless
Interceptor, and verifies JavaScript ingestion plus a lazy chunk downloaded by
jxscout through Gori. It requires no model API key and contacts no target site.
The test's temporary state is removed when it exits; failure output includes
service diagnostics.

Normal service logs remain in the engagement's `private/stack/` folder. An
unreachable extension fails startup explicitly. Review `chromium.log`, then
verify the unpacked extension and native messaging manifest are present. Gori
and jxscout failures identify their corresponding logs and stop the stack; Pi
preserves its saved engagement state.

Container termination sends signals to the supervisor, which stops its child
process groups and Interceptor daemon. The supervisor also watches launcher
liveness. After an ungraceful kill, follow the harness's existing lock and
interrupted-task reconciliation instructions before resuming.

The `--offline` harness option does not start the browser/proxy stack. Native
installations can continue using their existing optional Burp/Caido imports;
the Docker path configures Gori automatically.

### Orchis structured result export

Terminal lifecycle events include `resultFile` when session metadata is available. This points to the private, atomically written `private/service-result.json` inside that session, using schema 1. The export includes disposition, completed phases, verified task count, blockers, bounded summary, validated findings, and the controller-verified report artifacts. It is intended for a trusted local worker, not direct exposure to job submitters.

The exporter checks artifact hashes and proof references. It never parses report prose to invent findings. Missing/changed terminal reports fail the job. Input-required and interrupted work receive an explicitly integration-generated partial report without advancing controller gates. Different validated issues on one URL retain distinct finding identities.

Orchis consumes the structured findings and coverage through its existing PDF pipeline, saves the PDF automatically, and displays the report and summary inside Orchis. Raw HTTP/identity evidence and the validation ledger remain private; neither a Markdown viewer nor a Markdown download is part of the user workflow.

### Import preliminary scanner findings

`pentest-job --inputs-dir /private/scanner-inputs --prompt-file -` imports the two preliminary scanner observation files into the actual engagement workspace at `inputs/scanners/` before emitting `started` and sending the engagement prompt. The `started` event includes `scannerInputs` when supplied.

The source directory must contain a schema-1 `manifest.json` with exactly two `sources` entries for `nmap.json` and `nuclei.json`, each declaring `name`, `sha256` and `bytes`. Each observation file contains `tool`, terminal `status`, `target`, `params`, `findings` and optional `execution_id` / `error`. Files are private, bounded to 32 MiB each, hash-verified and copied without following symlinks. Changed existing imports are refused on continuation.

Imports supply untrusted reconnaissance observations; they do not grant target authorization or bypass workflow/evidence gates. Scanner errors stay explicit. Orchis uses them as leads for Hackbot and keeps the final PDF and summary in its native report interface.
