import contextlib
import io
import json
import os
from pathlib import Path
import tempfile
import types
import unittest
from unittest import mock

import mitre_config
import ops


class MitreConfiguration(unittest.TestCase):
    def test_default_is_local_and_uses_committed_pin(self):
        value = mitre_config.manifest(ops.REPO)
        self.assertEqual(mitre_config.defaults(ops.REPO), {
            'ORCHIS_MITRE_URL': '/opt/orchis/mitre/' + value['file'],
            'ORCHIS_MITRE_SHA256': value['sha256'],
        })

    def test_invalid_manifest_cannot_escape_runtime_directory(self):
        for changed in [{'file': '../outside'}, {'sha256': 'z' * 64}, {'version': '../20.0'}]:
            with self.subTest(changed=changed), tempfile.TemporaryDirectory() as root:
                repo = Path(root)
                (repo / 'deploy/mitre').mkdir(parents=True)
                value = mitre_config.manifest(ops.REPO)
                value.update(changed)
                (repo / 'deploy/mitre/manifest.json').write_text(json.dumps(value))
                with self.assertRaises(RuntimeError):
                    mitre_config.defaults(repo)

    @unittest.skipUnless(os.geteuid() == 0, 'Private container config belongs to UID10001')
    def test_explicit_upgrade_preserves_credentials_and_is_idempotent(self):
        with tempfile.TemporaryDirectory() as root:
            state = Path(root)
            for name in ['server.json', 'migrate.json', 'provision.json']:
                ops.private(state / name, {'ORCHIS_EXISTING_SECRET': 'unchanged', 'ORCHIS_MITRE_URL': 'off'})
            args = types.SimpleNamespace(state=state)
            with contextlib.redirect_stdout(io.StringIO()):
                mitre_config.configure(args, ops.REPO, ops.private)
                mitre_config.configure(args, ops.REPO, ops.private)
            for name in ['server.json', 'migrate.json', 'provision.json']:
                path = state / name
                value = json.loads(path.read_text())
                self.assertEqual(value['ORCHIS_EXISTING_SECRET'], 'unchanged')
                self.assertEqual(value['ORCHIS_MITRE_SHA256'], mitre_config.defaults(ops.REPO)['ORCHIS_MITRE_SHA256'])
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                self.assertEqual(path.stat().st_uid, 10001)

    @unittest.skipUnless(os.geteuid() == 0, 'Private container config belongs to UID10001')
    def test_invalid_config_is_rejected_before_any_file_changes(self):
        with tempfile.TemporaryDirectory() as root:
            state = Path(root)
            for name in ['server.json', 'migrate.json', 'provision.json']:
                ops.private(state / name, {'ORCHIS_MITRE_URL': 'off'})
            original = (state / 'server.json').read_bytes()
            (state / 'provision.json').chmod(0o644)
            with self.assertRaises(RuntimeError):
                mitre_config.configure(types.SimpleNamespace(state=state), ops.REPO, ops.private)
            self.assertEqual((state / 'server.json').read_bytes(), original)

    def test_probe_rejects_partial_or_wrong_bundle(self):
        expected = mitre_config.manifest(ops.REPO)
        result = {'imported': True, 'total': expected['counts']['techniques'], 'tactics': [],
                  'last_run': {'status': 'succeeded', 'source_version': expected['version'], 'source_sha256': expected['sha256']}}
        client = mock.Mock()
        client.open.return_value.__enter__ = lambda _: io.BytesIO(json.dumps(result).encode())
        client.open.return_value.__exit__ = lambda *args: None
        with self.assertRaises(RuntimeError):
            mitre_config.verify_import(client, 'https://localhost:8443', ops.REPO)
