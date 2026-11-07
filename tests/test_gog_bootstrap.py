import concurrent.futures
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "docker/gog-bootstrap.py"
SPEC = importlib.util.spec_from_file_location("gog_bootstrap", SCRIPT)
BOOTSTRAP = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(BOOTSTRAP)

SNAPSHOT = """import hashlib,json,os,sys
print(json.dumps({'password_hash':hashlib.sha256(os.environ['GOG_KEYRING_PASSWORD'].encode()).hexdigest(),
'args':sys.argv[1:],'home':os.environ['GOG_HOME'],'readonly':os.environ['GOG_READONLY'],
'no_send':os.environ['GOG_GMAIL_NO_SEND'],'wrap':os.environ['GOG_WRAP_UNTRUSTED']}))
"""

FAKE_GOG = r'''#!/usr/bin/env python3
import hashlib,json,os,sys
from pathlib import Path
args=sys.argv[1:]
client=args[args.index('--client')+1]
operation=args[args.index('--client')+2:]
record={'operation':operation,'password_hash':hashlib.sha256(os.environ['GOG_KEYRING_PASSWORD'].encode()).hexdigest()}
if operation[:3] in (['auth','credentials','set'],['auth','tokens','import']):
    record['input_mode']=Path(operation[3]).stat().st_mode & 0o777
with open(os.environ['FAKE_GOG_LOG'],'a') as stream:
    stream.write(json.dumps(record)+'\n')
if os.environ.get('FAKE_GOG_FAIL'):
    print('fixture-secret-from-stdout')
    print('fixture-secret-from-stderr',file=sys.stderr)
    sys.exit(7)
root=Path(os.environ['GOG_HOME'])/'data/keyring'
root.mkdir(parents=True,exist_ok=True)
state=root/'fake-tokens.json'
tokens=json.loads(state.read_text()) if state.exists() else {}
if operation[:3]==['auth','credentials','set']:
    json.loads(Path(operation[3]).read_text())
    (root/'fake-client-secret').write_text('fixture-only-client-marker')
    filename='credentials.json' if client=='default' else 'credentials-'+client+'.json'
    (root.parent/filename).write_text('{}')
    print('{}')
elif operation==['auth','tokens','list']:
    if os.environ.get('FAKE_GOG_ROTATE_TOKEN_FILE'):
        Path(os.environ['FAKE_GOG_ROTATE_TOKEN_FILE']).write_text(json.dumps({'email':'other@example.com','refresh_token':'rotated-fixture'}))
    print(json.dumps({'keys':list(tokens)}))
elif operation[:3]==['auth','tokens','import']:
    token=json.loads(Path(operation[3]).read_text())
    tokens['token:'+client+':'+token['email'].lower()]=token
    state.write_text(json.dumps(tokens))
    print('{}')
else:
    sys.exit(9)
'''


class GogBootstrapTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="gog-bootstrap-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        fake = self.bin / "gog"
        fake.write_text(FAKE_GOG)
        fake.chmod(0o755)
        self.env = {key: value for key, value in os.environ.items()
                    if not key.startswith("GOG_") and key != "DBUS_SESSION_BUS_ADDRESS"}
        self.env.update(HOME=str(self.home), PATH=str(self.bin) + os.pathsep + self.env.get("PATH", ""),
                        FAKE_GOG_LOG=str(self.root / "calls.jsonl"))
        self.gog_home = self.home / ".gog"

    def calls(self, operation=None):
        path = self.root / "calls.jsonl"
        rows = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        return [row for row in rows if row["operation"][:len(operation)] == operation] if operation else rows

    def client_file(self):
        path = self.root / "oauth client ; literal.json"
        path.write_text(json.dumps({"installed": {"client_id": "fixture.apps.googleusercontent.com",
                                                 "client_secret": "fixture-only-secret"}}))
        return path

    def token_file(self, email="tester@example.com", client="default"):
        path = self.root / "token-export.json"
        path.write_text(json.dumps({"email": email, "client": client, "services": ["gmail"],
                                    "refresh_token": "fixture-only-refresh"}))
        return path

    def run_command(self, env=None, *args):
        return subprocess.run([sys.executable, str(SCRIPT), "--", sys.executable, "-c", SNAPSHOT, *args],
                              env=env or self.env, capture_output=True, text=True, timeout=20)

    def token_store(self):
        return self.gog_home / "data/keyring/fake-tokens.json"

    def test_generated_password_is_persistent_private_and_no_auth_is_required(self):
        first = BOOTSTRAP.bootstrap(self.env)
        second = BOOTSTRAP.bootstrap(self.env)
        self.assertEqual(first["GOG_KEYRING_PASSWORD"], second["GOG_KEYRING_PASSWORD"])
        self.assertGreaterEqual(len(first["GOG_KEYRING_PASSWORD"]), 60)
        password_file = self.gog_home / "bootstrap/keyring-password"
        self.assertEqual(stat.S_IMODE(password_file.stat().st_mode), 0o600)
        self.assertEqual(stat.S_IMODE(self.gog_home.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(password_file.parent.stat().st_mode), 0o700)
        self.assertEqual(first["GOG_KEYRING_BACKEND"], "file")
        self.assertEqual(first["DBUS_SESSION_BUS_ADDRESS"], "disabled:")
        for key in ("GOG_READONLY", "GOG_GMAIL_NO_SEND", "GOG_WRAP_UNTRUSTED", "GOG_AUTO_JSON"):
            self.assertEqual(first[key], "1")
        self.assertNotIn("GOG_NO_INPUT", first)
        self.assertEqual(self.calls(), [])

    def test_explicit_password_and_flags_are_preserved_without_persisting_secret(self):
        env = dict(self.env, GOG_KEYRING_PASSWORD="  fixture password  ", GOG_READONLY="0", GOG_GMAIL_NO_SEND="false")
        result = BOOTSTRAP.bootstrap(env)
        self.assertEqual(result["GOG_KEYRING_PASSWORD"], env["GOG_KEYRING_PASSWORD"])
        self.assertEqual(result["GOG_READONLY"], "0")
        self.assertEqual(result["GOG_GMAIL_NO_SEND"], "false")
        self.assertFalse((self.gog_home / "bootstrap/keyring-password").exists())

    def test_password_mount_strips_one_line_ending_only_and_environment_wins(self):
        password = self.root / "password"
        password.write_bytes(b"  fixture password  \r\n")
        env = dict(self.env, GOG_KEYRING_PASSWORD_FILE=str(password))
        self.assertEqual(BOOTSTRAP.bootstrap(env)["GOG_KEYRING_PASSWORD"], "  fixture password  ")
        env["GOG_KEYRING_PASSWORD"] = "explicit takes precedence"
        env["GOG_KEYRING_PASSWORD_FILE"] = str(self.root / "missing")
        self.assertEqual(BOOTSTRAP.bootstrap(env)["GOG_KEYRING_PASSWORD"], "explicit takes precedence")

    def test_existing_keyring_without_saved_or_supplied_password_fails_closed(self):
        keyring = self.gog_home / "data/keyring"
        keyring.mkdir(parents=True)
        (keyring / "encrypted-entry").write_text("fixture-only-ciphertext-marker")
        with self.assertRaisesRegex(BOOTSTRAP.BootstrapError, "missing_unlock_secret"):
            BOOTSTRAP.bootstrap(self.env)
        self.assertFalse((self.gog_home / "bootstrap/keyring-password").exists())

    def test_legacy_keyring_without_saved_or_supplied_password_is_detected(self):
        keyring = self.gog_home / "config/keyring"
        keyring.mkdir(parents=True)
        (keyring / "encrypted-entry").write_text("fixture-only-ciphertext-marker")
        with self.assertRaisesRegex(BOOTSTRAP.BootstrapError, "missing_unlock_secret"):
            BOOTSTRAP.bootstrap(self.env)

    def test_client_hash_imports_once_and_changed_explicit_client_is_reimported(self):
        client = self.client_file()
        env = dict(self.env, GOG_OAUTH_CLIENT_FILE=str(client))
        BOOTSTRAP.bootstrap(env)
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(len(self.calls(["auth", "credentials", "set"])), 1)
        payload = json.loads(client.read_text())
        payload["installed"]["client_secret"] = "changed-fixture-secret"
        client.write_text(json.dumps(payload))
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(len(self.calls(["auth", "credentials", "set"])), 2)
        receipts = (self.gog_home / "bootstrap/receipts.json").read_text()
        self.assertNotIn("fixture-secret", receipts)
        self.assertNotIn("client_secret", receipts)
        self.assertTrue(all(row["input_mode"] == 0o600 for row in self.calls(["auth", "credentials", "set"])))

    def test_explicit_import_pins_default_client_but_preserves_named_and_no_import_selection(self):
        env = dict(self.env, GOG_OAUTH_CLIENT_FILE=str(self.client_file()))
        self.assertEqual(BOOTSTRAP.bootstrap(env)["GOG_CLIENT"], "default")
        env["GOG_CLIENT"] = "   "
        self.assertEqual(BOOTSTRAP.bootstrap(env)["GOG_CLIENT"], "default")
        env["GOG_CLIENT"] = "work"
        self.assertEqual(BOOTSTRAP.bootstrap(env)["GOG_CLIENT"], "work")
        self.assertNotIn("GOG_CLIENT", BOOTSTRAP.bootstrap(self.env))

    def test_matching_client_receipt_recovers_deleted_client_metadata(self):
        env = dict(self.env, GOG_OAUTH_CLIENT_FILE=str(self.client_file()))
        BOOTSTRAP.bootstrap(env)
        (self.gog_home / "data/credentials.json").unlink()
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(len(self.calls(["auth", "credentials", "set"])), 2)

    def test_token_import_is_once_and_preserves_refreshed_or_changed_exports(self):
        token = self.token_file()
        env = dict(self.env, GOG_ACCOUNT="tester@example.com", GOG_TOKEN_FILE=str(token))
        BOOTSTRAP.bootstrap(env)
        store = json.loads(self.token_store().read_text())
        store["token:default:tester@example.com"]["refresh_token"] = "refreshed-in-live-store"
        self.token_store().write_text(json.dumps(store))
        BOOTSTRAP.bootstrap(env)
        payload = json.loads(token.read_text())
        payload["refresh_token"] = "changed-explicit-fixture"
        token.write_text(json.dumps(payload))
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(len(self.calls(["auth", "tokens", "import"])), 1)
        self.assertEqual(json.loads(self.token_store().read_text())["token:default:tester@example.com"]["refresh_token"], "refreshed-in-live-store")
        env["GOG_TOKEN_IMPORT_REPLACE"] = "1"
        BOOTSTRAP.bootstrap(env)
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(len(self.calls(["auth", "tokens", "import"])), 2)
        self.assertEqual(json.loads(self.token_store().read_text())["token:default:tester@example.com"]["refresh_token"], "changed-explicit-fixture")

    def test_matching_receipt_recovers_when_explicit_token_was_deleted(self):
        token = self.token_file()
        env = dict(self.env, GOG_ACCOUNT="tester@example.com", GOG_TOKEN_FILE=str(token))
        BOOTSTRAP.bootstrap(env)
        self.token_store().write_text("{}")
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(len(self.calls(["auth", "tokens", "import"])), 2)

    def test_existing_unmanaged_token_is_not_clobbered_by_new_mount(self):
        BOOTSTRAP.bootstrap(self.env)
        self.token_store().parent.mkdir(parents=True, exist_ok=True)
        self.token_store().write_text(json.dumps({"token:default:tester@example.com": {"refresh_token": "existing-fixture-token"}}))
        env = dict(self.env, GOG_ACCOUNT="tester@example.com", GOG_TOKEN_FILE=str(self.token_file()))
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(self.calls(["auth", "tokens", "import"]), [])

    def test_legacy_default_token_key_is_preserved(self):
        BOOTSTRAP.bootstrap(self.env)
        self.token_store().parent.mkdir(parents=True, exist_ok=True)
        self.token_store().write_text(json.dumps({"token:tester@example.com": {"refresh_token": "legacy-fixture-token"}}))
        env = dict(self.env, GOG_ACCOUNT="tester@example.com", GOG_TOKEN_FILE=str(self.token_file()))
        BOOTSTRAP.bootstrap(env)
        self.assertEqual(self.calls(["auth", "tokens", "import"]), [])

    def test_import_uses_validated_snapshot_when_original_mount_rotates(self):
        token = self.token_file()
        env = dict(self.env, GOG_ACCOUNT="tester@example.com", GOG_TOKEN_FILE=str(token), FAKE_GOG_ROTATE_TOKEN_FILE=str(token))
        BOOTSTRAP.bootstrap(env)
        self.assertIn("token:default:tester@example.com", json.loads(self.token_store().read_text()))
        self.assertNotIn("token:default:other@example.com", json.loads(self.token_store().read_text()))
        self.assertTrue(all(row["input_mode"] == 0o600 for row in self.calls(["auth", "tokens", "import"])))
        self.assertEqual(list((self.gog_home / "bootstrap").glob(".gog-import-*")), [])

    def test_account_and_client_mismatch_or_missing_account_are_rejected(self):
        token = self.token_file()
        for updates in ({"GOG_ACCOUNT": "other@example.com"}, {}, {"GOG_ACCOUNT": "tester@example.com", "GOG_CLIENT": "work"}):
            with self.subTest(updates=updates):
                with self.assertRaises(BOOTSTRAP.BootstrapError):
                    BOOTSTRAP.bootstrap(dict(self.env, GOG_TOKEN_FILE=str(token), **updates))
        self.assertEqual(self.calls(), [])

    def test_all_supplied_inputs_are_validated_before_local_imports(self):
        env = dict(self.env, GOG_OAUTH_CLIENT_FILE=str(self.client_file()), GOG_TOKEN_FILE=str(self.token_file()), GOG_ACCOUNT="wrong@example.com")
        with self.assertRaises(BOOTSTRAP.BootstrapError):
            BOOTSTRAP.bootstrap(env)
        self.assertEqual(self.calls(), [])

    def test_failed_gog_output_is_redacted_and_failed_import_has_no_receipt(self):
        env = dict(self.env, GOG_OAUTH_CLIENT_FILE=str(self.client_file()), FAKE_GOG_FAIL="1")
        result = self.run_command(env)
        self.assertEqual(result.returncode, 1)
        self.assertIn("OAuth client import failed", result.stderr)
        self.assertNotIn("fixture-secret", result.stdout + result.stderr)
        self.assertNotIn("Traceback", result.stderr)
        self.assertFalse((self.gog_home / "bootstrap/receipts.json").exists())

    def test_concurrent_jobs_share_one_password_and_import_once(self):
        env = dict(self.env, GOG_ACCOUNT="tester@example.com", GOG_TOKEN_FILE=str(self.token_file()))
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
            results = list(executor.map(lambda _index: self.run_command(env), range(4)))
        self.assertEqual([result.returncode for result in results], [0] * 4)
        self.assertEqual(len({json.loads(result.stdout)["password_hash"] for result in results}), 1)
        self.assertEqual(len(self.calls(["auth", "tokens", "import"])), 1)

    def test_exec_preserves_arguments_without_shell_expansion(self):
        marker = self.root / "must-not-be-created"
        argument = "$(touch " + str(marker) + ") ; literal"
        result = self.run_command(None, argument, "spaces and 'quotes'")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["args"], [argument, "spaces and 'quotes'"])
        self.assertFalse(marker.exists())

    def test_status_is_read_only_and_never_claims_authentication(self):
        result = subprocess.run([sys.executable, str(SCRIPT), "--status"], env=self.env, capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["authentication"], "not_checked")
        self.assertFalse(self.gog_home.exists())


if __name__ == "__main__":
    unittest.main()
