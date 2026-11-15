---
name: gog-gmail
description: "Gmail operations through gog — read verification emails, OTP codes, and reset links for test accounts."
---

# Gmail

Use `gog` for Gmail operations.

Shared rules: select the account with `--account`; use `--json --wrap-untrusted`
for agent-readable output; `--readonly` when nothing may mutate; `--no-input`
in automation; `--dry-run` before writes; confirm the exact account/object
before any write or delete.

## Docker bootstrap and account authorization

The Docker entrypoint prepares gog before spawning the harness or service job.
It defaults `GOG_HOME` to the persistent `/home/pentest/.gog`, uses the encrypted
file keyring, disables desktop D-Bus autostart, and defaults `GOG_READONLY`,
`GOG_GMAIL_NO_SEND`, and `GOG_WRAP_UNTRUSTED` to `1` while preserving explicit
operator overrides. Its keyring password is generated once in private persistent
storage unless supplied through `GOG_KEYRING_PASSWORD` or a mounted
`GOG_KEYRING_PASSWORD_FILE`.

Only explicit `GOG_OAUTH_CLIENT_FILE` and `GOG_TOKEN_FILE` mounts are imported.
Token import requires `GOG_ACCOUNT` matching the token email. The bootstrap uses
local imports and locked receipts; it does not check online connectivity, fetch
mail, initiate consent, or discover a host mailbox. Existing tokens are reused,
not proof of a working login. With no authorized tokens, record Gmail-dependent
coverage as blocked and continue independent work.

First OAuth consent is an operator-only setup task: use the same persistent home,
OAuth client, and account for `gog auth add <email> --services gmail
--gmail-scope readonly --remote --step 1`, then step 2 with `--auth-url` and the
full browser redirect URL. Do not ask the model to receive the consent URL,
redirect URL, client secret, or refresh-token export. Preserve the mapped
engagement account; never authorize an unrelated account as a fallback.

Inspect local `gog auth list --json --no-input` first. An online `--check` or
Gmail search is appropriate only when the assigned task authorizes use of that
mailbox; container startup alone does not do either. Follow the pinned upstream
[paths](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/paths.md),
[OAuth clients](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/auth-clients.md),
and [token import](https://github.com/openclaw/gogcli/blob/v0.48.0/docs/commands/gog-auth-tokens-import.md)
contracts if diagnosing setup failures.

## Safe start

```bash
gog auth list --check --json --no-input
gog schema gmail --json
gog --readonly --account user@example.com gmail search 'newer_than:7d' --max 10 --json --wrap-untrusted
```

- Select the account explicitly with `--account`.
- Use `--json --wrap-untrusted` for agent-readable Google content.
- Use `--readonly` when the task must not mutate Google data.
- Use `--no-input` in automation and `--dry-run` before supported writes.
- Confirm the exact account, object, and mutation before any write or delete.

## Commands

| Command | Purpose |
| --- | --- |
| `archive` | Archive messages or explicit threads (remove from inbox) |
| `attachment` | Download a single attachment |
| `autoreply` | Reply once to matching messages |
| `batch` | Batch operations (permanent delete requires broader Gmail scope; use gmail trash for normal trashing) |
| `drafts` | Draft operations |
| `forward` | Forward a message to new recipients |
| `get` | Get a message (full\|metadata\|raw) |
| `history` | Gmail history |
| `import` | Import an RFC822/EML message into Gmail |
| `labels` | Label operations |
| `mark-read` | Mark messages as read |
| `messages` | Message operations |
| `raw` | Dump raw Gmail API response as JSON (Users.Messages.Get; lossless; for scripting and LLM consumption) |
| `reply` | Reply to a message |
| `reply-all` | Reply to all message participants |
| `search` | Search threads using Gmail query syntax |
| `send` | Send an email |
| `settings` | Settings and admin |
| `thread` | Thread operations (get, modify) |
| `track` | Email open tracking |
| `trash` | Move messages to trash |
| `unread` | Mark messages as unread |
| `url` | Print Gmail web URLs for threads |

Run `gog gmail <command> --help` for flags and `gog schema gmail <command> --json`
for the machine-readable contract. Do not guess command syntax.