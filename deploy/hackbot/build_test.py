"""Exercise source validation with real temporary Git checkouts; Docker is mocked."""
import hashlib
import importlib.util
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

spec = importlib.util.spec_from_file_location('engine_build', Path(__file__).with_name('build.py'))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)


class EngineSources(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'orchis'
        (self.repo / 'deploy/hackbot').mkdir(parents=True)
        self.pins = []
        for directory in ['orchis-hackbot-lite', 'orchis-opencode-hackbot']:
            source = self.root / directory
            source.mkdir()
            (source / 'entry.mjs').write_text('export {};\n')
            self.git(source, 'init', '-q')
            self.git(source, 'add', '.')
            self.git(source, '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                     '-c', 'commit.gpgsign=false', 'commit', '-qm', 'fixture')
            self.pins.append({'repository': 'https://example.invalid/' + directory,
                              'revision': self.git(source, 'rev-parse', 'HEAD'),
                              'files': {'entry.mjs': hashlib.sha256((source / 'entry.mjs').read_bytes()).hexdigest()}})
        self.lock = self.repo / 'deploy/hackbot/engine-lock.json'
        self.save_lock()
        self.environment = mock.patch.dict(os.environ)
        self.environment.start()
        self.addCleanup(self.environment.stop)
        for key in ['ORCHIS_HACKBOT_SOURCE', 'ORCHIS_OPENCODE_SOURCE']:
            os.environ.pop(key, None)

    def git(self, source, *args):
        return subprocess.check_output(['git', '-C', str(source), *args], text=True, stderr=subprocess.PIPE).strip()

    def save_lock(self):
        self.lock.write_text(json.dumps({**self.pins[0], 'opencode': self.pins[1]}))

    def test_matching_checkouts_and_overridden_location(self):
        custom = self.root / 'custom'
        (self.root / 'orchis-opencode-hackbot').rename(custom)
        os.environ['ORCHIS_OPENCODE_SOURCE'] = str(custom)
        self.assertEqual(build.check_sources(self.repo)[1], custom)

    def test_missing_second_source_prevents_every_docker_build(self):
        os.environ['ORCHIS_OPENCODE_SOURCE'] = str(self.root / 'absent')
        original = build.check_sources
        with mock.patch.object(build, 'check_sources', side_effect=lambda: original(self.repo)), \
             mock.patch.object(build, 'build_images') as docker:
            with self.assertRaises(SystemExit):
                build.main([])
            docker.assert_not_called()

    def test_wrong_revision_and_missing_or_changed_files_are_rejected(self):
        source = self.root / 'orchis-opencode-hackbot'
        self.pins[1]['revision'] = '0' * 40
        self.save_lock()
        with self.assertRaisesRegex(build.SourceError, 'revision mismatch'):
            build.check_sources(self.repo)
        self.pins[1]['revision'] = self.git(source, 'rev-parse', 'HEAD')
        self.save_lock()
        (source / 'entry.mjs').unlink()
        with self.assertRaisesRegex(build.SourceError, 'file is missing'):
            build.check_sources(self.repo)
        (source / 'entry.mjs').write_text('modified')
        with self.assertRaisesRegex(build.SourceError, 'differs from engine-lock'):
            build.check_sources(self.repo)

    def test_archive_without_git_metadata_is_rejected(self):
        source = self.root / 'archive'
        source.mkdir()
        os.environ['ORCHIS_OPENCODE_SOURCE'] = str(source)
        with self.assertRaisesRegex(build.SourceError, 'must be a Git checkout'):
            build.check_sources(self.repo)

    def test_check_mode_never_runs_docker(self):
        with mock.patch.object(build, 'check_sources', return_value=['lite', 'opencode']), \
             mock.patch.object(build, 'build_images') as docker:
            build.main(['--check'])
            docker.assert_not_called()


if __name__ == '__main__':
    unittest.main()
