#!/usr/bin/env python3
"""Configure the persistent, unattended gog v0.48 file keyring, then exec argv.

Only explicitly supplied GOG_OAUTH_CLIENT_FILE / GOG_TOKEN_FILE are imported.
Token JSON must be a `gog auth tokens export` result for GOG_ACCOUNT and
GOG_CLIENT (default: default). Existing tokens are preserved unless a changed
input is explicitly authorized with GOG_TOKEN_IMPORT_REPLACE=1. No OAuth or
Google API requests are made here.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import stat
import subprocess
import sys
import tempfile


class BootstrapError(Exception):
    """Messages are fixed explanations; never include subprocess/file contents."""


def home_path(env):
    value = env.get("GOG_HOME", str(Path(env.get("HOME", str(Path.home()))) / ".gog"))
    path = Path(value)
    if not path.is_absolute():
        raise BootstrapError("GOG_HOME must be an absolute private directory")
    for name in ("GOG_CONFIG_DIR", "GOG_DATA_DIR", "GOG_STATE_DIR", "GOG_CACHE_DIR"):
        if name in env and not Path(env[name]).is_absolute():
            raise BootstrapError("GOG directory overrides must be absolute paths")
    return path


def private_directory(path):
    try:
        path.mkdir(parents=True, mode=0o700, exist_ok=True)
        info = path.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid():
            raise BootstrapError("gog state directory must be owned by the container user and cannot be a symlink")
        path.chmod(0o700)
    except OSError:
        raise BootstrapError("cannot prepare the private gog state directory") from None


def read_input(path, label, limit=4 * 1024 * 1024):
    """Explicit secret mounts may be symlinks; their opened target must be a file."""
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
        with os.fdopen(descriptor, "rb") as stream:
            if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
                raise BootstrapError(label + " must refer to a regular file")
            contents = stream.read(limit + 1)
        if len(contents) > limit:
            raise BootstrapError(label + " is too large")
        return contents
    except (OSError, ValueError):
        raise BootstrapError("cannot read " + label) from None


def read_private(path, label):
    try:
        descriptor = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
        with os.fdopen(descriptor, "rb") as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid():
                raise BootstrapError(label + " must be a private file owned by the container user")
            os.fchmod(stream.fileno(), 0o600)
            contents = stream.read(4 * 1024 * 1024 + 1)
            if len(contents) > 4 * 1024 * 1024:
                raise BootstrapError(label + " is too large")
            return contents
    except FileNotFoundError:
        return None
    except OSError:
        raise BootstrapError("cannot read " + label) from None


def atomic_write(path, contents):
    temporary = None
    try:
        descriptor, temporary = tempfile.mkstemp(prefix=".gog-bootstrap-", dir=path.parent)
        with os.fdopen(descriptor, "wb") as stream:
            os.fchmod(stream.fileno(), 0o600)
            stream.write(contents)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        temporary = None
        directory = os.open(path.parent, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    except OSError:
        raise BootstrapError("cannot persist private gog bootstrap state") from None
    finally:
        if temporary:
            try:
                os.unlink(temporary)
            except OSError:
                pass


def password_text(contents):
    try:
        value = contents.decode("utf-8")
    except UnicodeError:
        raise BootstrapError("keyring password file must contain UTF-8 text") from None
    if value.endswith("\r\n"):
        value = value[:-2]
    elif value.endswith(("\n", "\r")):
        value = value[:-1]
    if not value or "\x00" in value:
        raise BootstrapError("keyring password must be nonempty and contain no NUL characters")
    return value


def existing_keyring(env, home):
    # The v0.48 GOG_HOME data layout is flat. Explicit DATA_DIR wins, as in gog.
    directories = [Path(env.get("GOG_DATA_DIR", str(home / "data"))) / "keyring"]
    if "GOG_DATA_DIR" not in env:
        directories.append(Path(env.get("GOG_CONFIG_DIR", str(home / "config"))) / "keyring")
    try:
        return any(directory.is_dir() and any(item.name != ".lock" for item in directory.iterdir()) for directory in directories)
    except OSError:
        raise BootstrapError("cannot inspect the existing gog file keyring") from None


def prepare_password(env, home, bootstrap):
    if "GOG_KEYRING_PASSWORD" in env:
        password = env["GOG_KEYRING_PASSWORD"]
        if not password or "\x00" in password:
            raise BootstrapError("GOG_KEYRING_PASSWORD must be nonempty and contain no NUL characters")
        return "environment"
    if "GOG_KEYRING_PASSWORD_FILE" in env:
        env["GOG_KEYRING_PASSWORD"] = password_text(read_input(env["GOG_KEYRING_PASSWORD_FILE"], "GOG_KEYRING_PASSWORD_FILE", 16384))
        return "file"
    saved = read_private(bootstrap / "keyring-password", "saved gog keyring password")
    if saved is not None:
        env["GOG_KEYRING_PASSWORD"] = password_text(saved)
        return "generated"
    if existing_keyring(env, home):
        raise BootstrapError("missing_unlock_secret: existing gog file keyring requires GOG_KEYRING_PASSWORD or GOG_KEYRING_PASSWORD_FILE")
    password = secrets.token_urlsafe(48)
    atomic_write(bootstrap / "keyring-password", (password + "\n").encode())
    env["GOG_KEYRING_PASSWORD"] = password
    return "generated"


def json_object(contents, label):
    try:
        result = json.loads(contents)
        if not isinstance(result, dict):
            raise ValueError()
        return result
    except (ValueError, UnicodeError):
        raise BootstrapError(label + " must contain a JSON object") from None


def selected_client(env):
    client = env.get("GOG_CLIENT", "default").strip().lower() or "default"
    if not re.fullmatch(r"[a-z0-9][a-z0-9_.-]{0,127}", client):
        raise BootstrapError("GOG_CLIENT must be a valid OAuth client name")
    return client


def gog(env, client, operation, action):
    args = ["gog", "--no-input", "--json", "--client", client, *operation]
    try:
        result = subprocess.run(args, env=env, stdin=subprocess.DEVNULL, capture_output=True, timeout=25)
    except (OSError, ValueError, subprocess.SubprocessError):
        raise BootstrapError("gog local " + action + " could not run; verify the installed CLI and supplied configuration") from None
    if result.returncode:
        # Upstream errors may echo malformed token/date/client content. Never
        # copy subprocess output, arguments, or exception repr into job logs.
        raise BootstrapError("gog local " + action + " failed; verify supplied files and keyring configuration")
    return result.stdout


def receipt_key(kind, client, account=""):
    return hashlib.sha256((kind + "\0" + client + "\0" + account.casefold()).encode()).hexdigest()


def import_snapshot(env, client, bootstrap, contents, operation, action):
    # Secret mounts can rotate between validation and the subprocess read.
    # Give gog the exact bytes whose account/client/hash were checked above.
    temporary = None
    try:
        descriptor, temporary = tempfile.mkstemp(prefix=".gog-import-", suffix=".json", dir=bootstrap)
        with os.fdopen(descriptor, "wb") as stream:
            os.fchmod(stream.fileno(), 0o600)
            stream.write(contents)
            stream.flush()
            os.fsync(stream.fileno())
        gog(env, client, [*operation, temporary], action)
    except OSError:
        raise BootstrapError("cannot prepare the private gog import snapshot") from None
    finally:
        if temporary:
            try:
                os.unlink(temporary)
            except OSError:
                pass


def client_metadata_present(env, home, client):
    filename = "credentials.json" if client == "default" else "credentials-" + client + ".json"
    roots = [Path(env.get("GOG_DATA_DIR", str(home / "data"))), Path(env.get("GOG_CONFIG_DIR", str(home / "config")))]
    return any((root / filename).is_file() for root in roots)


def import_explicit(env, home, bootstrap):
    path = bootstrap / "receipts.json"
    contents = read_private(path, "gog import receipts")
    receipts = json_object(contents, "gog import receipts") if contents is not None else {}
    client = selected_client(env)
    changed = False
    client_file = env.get("GOG_OAUTH_CLIENT_FILE")
    token_file = env.get("GOG_TOKEN_FILE")
    client_data = None
    token_data = None
    if client_file is not None:
        client_data = read_input(client_file, "GOG_OAUTH_CLIENT_FILE")
        payload = json_object(client_data, "GOG_OAUTH_CLIENT_FILE")
        credentials = payload.get("installed", payload.get("web", payload))
        if not isinstance(credentials, dict) or not all(isinstance(credentials.get(key), str) and credentials[key].strip()
                                                     for key in ("client_id", "client_secret")):
            raise BootstrapError("GOG_OAUTH_CLIENT_FILE must contain Google OAuth client credentials")
    if token_file is not None:
        token_data = read_input(token_file, "GOG_TOKEN_FILE")
        payload = json_object(token_data, "GOG_TOKEN_FILE")
        account = env.get("GOG_ACCOUNT", "").strip().casefold()
        email = payload.get("email")
        if not account or account.count("@") != 1 or any(char.isspace() for char in account):
            raise BootstrapError("GOG_ACCOUNT must name the exact mailbox when GOG_TOKEN_FILE is supplied")
        if not isinstance(email, str) or email.strip().casefold() != account:
            raise BootstrapError("GOG_TOKEN_FILE account does not match GOG_ACCOUNT")
        if not isinstance(payload.get("refresh_token"), str) or not payload["refresh_token"].strip():
            raise BootstrapError("GOG_TOKEN_FILE must be a gog token export containing refresh_token")
        token_client = payload.get("client", "default") or "default"
        if not isinstance(token_client, str) or token_client.strip().lower() != client:
            raise BootstrapError("GOG_TOKEN_FILE client does not match GOG_CLIENT")

    # Validate every explicit input before making any local credential writes.
    if client_data is not None:
        digest = hashlib.sha256(client_data).hexdigest()
        key = receipt_key("client", client)
        if receipts.get(key, {}).get("sha256") != digest or not client_metadata_present(env, home, client):
            import_snapshot(env, client, bootstrap, client_data, ["auth", "credentials", "set"], "OAuth client import")
            receipts[key] = {"sha256": digest, "status": "imported"}
            changed = True
    if token_data is not None:
        digest = hashlib.sha256(token_data).hexdigest()
        key = receipt_key("token", client, account)
        previous = receipts.get(key, {})
        keys = json_object(gog(env, client, ["auth", "tokens", "list"], "token inventory"), "gog token inventory").get("keys")
        if not isinstance(keys, list) or not all(isinstance(item, str) for item in keys):
            raise BootstrapError("gog local token inventory returned an unsupported result")
        expected = {("token:" + client + ":" + account).casefold()}
        if client == "default":
            expected.add(("token:" + account).casefold())
        present = any(item.casefold() in expected for item in keys)
        replace = env.get("GOG_TOKEN_IMPORT_REPLACE", "").lower() in {"1", "true", "yes"}
        already_imported = previous.get("sha256") == digest and previous.get("status") == "imported"
        if not present or (replace and not already_imported):
            import_snapshot(env, client, bootstrap, token_data, ["auth", "tokens", "import"], "token import")
            receipts[key] = {"sha256": digest, "status": "imported"}
            changed = True
        elif not already_imported:
            # An old export must never replace a refreshed or manually installed
            # token merely because the same secret mount appears on every boot.
            receipts[key] = {"sha256": digest, "status": "existing_preserved"}
            changed = True
    if changed:
        atomic_write(path, (json.dumps(receipts, sort_keys=True) + "\n").encode())


def bootstrap(environ=None):
    env = dict(os.environ if environ is None else environ)
    home = home_path(env)
    env.setdefault("GOG_HOME", str(home))
    for key, value in {"GOG_KEYRING_BACKEND": "file", "DBUS_SESSION_BUS_ADDRESS": "disabled:",
                       "GOG_READONLY": "1", "GOG_GMAIL_NO_SEND": "1", "GOG_WRAP_UNTRUSTED": "1",
                       "GOG_AUTO_JSON": "1"}.items():
        env.setdefault(key, value)
    if ("GOG_OAUTH_CLIENT_FILE" in env or "GOG_TOKEN_FILE" in env) and not env.get("GOG_CLIENT", "").strip():
        # The imported default bucket must also be the bucket subsequent jobs
        # select; an old account/domain mapping must not silently redirect it.
        env["GOG_CLIENT"] = "default"
    private_directory(home)
    directory = home / "bootstrap"
    private_directory(directory)
    try:
        descriptor = os.open(directory / "lock", os.O_RDWR | os.O_CREAT | getattr(os, "O_NOFOLLOW", 0), 0o600)
        with os.fdopen(descriptor, "a") as lock:
            info = os.fstat(lock.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid():
                raise BootstrapError("gog bootstrap lock must be owned by the container user")
            os.fchmod(lock.fileno(), 0o600)
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
            prepare_password(env, home, directory)
            if "GOG_OAUTH_CLIENT_FILE" in env or "GOG_TOKEN_FILE" in env:
                import_explicit(env, home, directory)
    except OSError:
        raise BootstrapError("cannot lock the private gog bootstrap state") from None
    return env


def status(environ=None):
    """Read-only local configuration status. Never open/decrypt the keyring."""
    env = os.environ if environ is None else environ
    home = home_path(env)
    return {"home": str(home), "backend": env.get("GOG_KEYRING_BACKEND", "file"),
            "account_selected": bool(env.get("GOG_ACCOUNT")),
            "password_supplied": bool(env.get("GOG_KEYRING_PASSWORD") or env.get("GOG_KEYRING_PASSWORD_FILE")),
            "generated_password_present": (home / "bootstrap/keyring-password").is_file(),
            "oauth_client_file_supplied": "GOG_OAUTH_CLIENT_FILE" in env,
            "token_file_supplied": "GOG_TOKEN_FILE" in env,
            "authentication": "not_checked"}


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    try:
        if args == ["--status"]:
            print(json.dumps(status()))
            return 0
        if len(args) < 2 or args[0] != "--":
            raise BootstrapError("usage: gog-bootstrap.py -- COMMAND [ARG ...] or --status")
        env = bootstrap()
        try:
            os.execvpe(args[1], args[1:], env)
        except (OSError, ValueError):
            raise BootstrapError("could not execute the requested container command") from None
    except BootstrapError as exc:
        print("gog bootstrap: " + str(exc), file=sys.stderr)
        return 1
    except Exception:
        print("gog bootstrap: local configuration failed; inspect the supplied configuration privately", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
