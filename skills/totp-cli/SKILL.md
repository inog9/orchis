---
name: totp-cli
description: Generate authenticator TOTP codes using the installed yitsushi/totp-cli when an authorized website login requests authenticator-based 2FA. Use existing account tokens or an explicitly supplied seed; distinguish TOTP from email/SMS codes, recovery codes, and passkeys.
license: MIT
metadata:
  version: "1.0"
---

# Authenticator codes with totp-cli

Use this skill when an authorized sign-in asks for a time-based authenticator
code. Continue the website flow in its existing browser session. Email/SMS OTPs,
push approval, recovery codes, and passkeys use their own channels; this tool
does not retrieve those codes or bypass MFA.

## Check the installed command first

```sh
command -v totp-cli
totp-cli --version
totp-cli generate --help
```

The locally checked build reports `dev`; use installed help as the command
contract rather than inferring capabilities from a release number. The version
option is `--version`, not a `version` subcommand. If missing, report the missing
dependency instead of silently installing a different authenticator.

## Existing token: normal sign-in

1. Confirm the intended website, account, and currently pending authenticator
   challenge. Use the engagement's account mapping; do not select a similarly
   named token from another account or environment.
2. If the mapping is unknown, list namespaces and then the relevant namespace:

   ```sh
   totp-cli list
   totp-cli list engagement
   ```

   `engagement` and `test-user` below are example names, not existing account
   mappings. Listing may require unlocking storage. Use an already configured
   secure unlock mechanism or have the user unlock it. Do not guess the password
   or place it in command arguments, source files, shell history, or reports.
3. Generate one code for the exact namespace/account pair:

   ```sh
   totp-cli generate --show-remaining engagement test-user
   ```

4. Submit the code promptly into the real website's authenticator field using
   the browser tool required by the active workflow. Preserve leading zeros.
   Remaining seconds are separate from the code; a configured account prefix
   can also affect output. Match the website's required format rather than
   extracting every digit from the whole output line.
5. Verify the resulting authenticated page or server response. A generated code
   or form submission alone does not establish successful login. Record the
   account label and outcome, with sensitive values removed.

Avoid continuous `--follow` output for a single challenge. If the code is near
expiry, use a fresh window before submission. If rejected, check the account,
expiry, and issuer settings; allow at most one fresh-code retry unless a separate
authorized MFA test calls for more. Generating again in the same time window may
give the same code. Stop on lockout/rate-limit warnings. If clock skew is suspected,
report it; do not change system time or try codes from many time windows.

## Explicitly supplied seed: instant mode

Inspect `totp-cli instant --help` first. Instant mode accepts the seed through
stdin or an existing `TOTP_TOKEN` environment variable without using saved
storage. Prefer an authorized secret provider piping the seed directly to stdin;
do not paste a real seed into a shell command, transcript, or temporary file.

Supported local options include `--length`, `--algorithm`, `--time-period`, and
`--show-remaining`. Match the issuer's parameters; the installed defaults are
six digits, SHA-1, and 30 seconds. An existing environment variable is not
permission to use its token for another account. If no safe input channel is
available, ask the user to provide the current code through the website instead.

## Enrollment and storage changes

Creating/importing a token or enrolling a new website factor is a separate task
from generating an existing code. Do it only when that account setup is within
the user's requested scope. Inspect `totp-cli add-token --help` before enrollment;
it accepts namespace/account and issuer-specific length, algorithm, and period.
Use interactive secret entry or the established secure input channel. Do not
overwrite an existing token or change/delete credentials as a login fallback.

Do not use `dump` for discovery: it exports token material. Respect an existing
`TOTP_CLI_CREDENTIAL_FILE` setting and do not copy the credential store into the
project. Seeds, QR provisioning URLs, vault passwords, recovery codes, and live
OTPs must stay out of reports, screenshots, Git commits, and bundled skills.

Upstream: [yitsushi/totp-cli](https://github.com/yitsushi/totp-cli). Commands and
options above were checked against the installed CLI on 2026-10-31; no saved
tokens were listed, generated, exported, or changed during that check.
