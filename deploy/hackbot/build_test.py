"""Exercise monorepo source validation without Git metadata; Docker is mocked."""
import hashlib
import importlib.util
import json
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
        self.repo = Path(self.temp.name) / 'orchis'
        (self.repo / 'deploy/hackbot').mkdir(parents=True)
        self.pins = []
        for directory in ['hackbot-lite', 'opencode-hackbot']:
            source = self.repo / 'engines' / directory
            source.mkdir(parents=True)
            (source / 'entry.mjs').write_text('export {};\n')
            self.pins.append({'repository': 'https://example.invalid/' + directory,
                              'revision': 'original-upstream-revision',
                              'files': {'entry.mjs': hashlib.sha256((source / 'entry.mjs').read_bytes()).hexdigest()}})
        (self.repo / 'deploy/hackbot/engine-lock.json').write_text(json.dumps({**self.pins[0], 'opencode': self.pins[1]}))

    def test_bundled_sources_need_no_sibling_repositories_or_git(self):
        self.assertFalse((self.repo / '.git').exists())
        self.assertEqual(build.check_sources(self.repo), [self.repo / 'engines/hackbot-lite', self.repo / 'engines/opencode-hackbot'])

    def test_missing_second_source_prevents_every_docker_build(self):
        (self.repo / 'engines/opencode-hackbot').rename(self.repo / 'removed')
        original = build.check_sources
        with mock.patch.object(build, 'check_sources', side_effect=lambda: original(self.repo)), \
             mock.patch.object(build, 'build_images') as docker:
            with self.assertRaises(SystemExit):
                build.main([])
            docker.assert_not_called()

    def test_missing_or_changed_integration_file_is_rejected(self):
        path = self.repo / 'engines/opencode-hackbot/entry.mjs'
        path.unlink()
        with self.assertRaisesRegex(build.SourceError, 'file is missing'):
            build.check_sources(self.repo)
        path.write_text('modified')
        with self.assertRaisesRegex(build.SourceError, 'differs from engine-lock'):
            build.check_sources(self.repo)

    def test_images_build_in_dependency_order_from_bundled_sources(self):
        sources = build.check_sources(self.repo)
        with mock.patch.object(build.subprocess, 'run') as docker:
            build.build_images(sources)
        self.assertEqual(docker.call_args_list, [
            mock.call(['docker', 'build', '--tag', 'orchis-hackbot-engine:orchis-v1', str(sources[0])], check=True),
            mock.call(['docker', 'build', '--tag', 'orchis-opencode-engine:orchis-v1', str(sources[1])], check=True),
        ])

    def test_check_mode_never_runs_docker(self):
        with mock.patch.object(build, 'check_sources', return_value=['lite', 'opencode']), \
             mock.patch.object(build, 'build_images') as docker:
            build.main(['--check'])
            docker.assert_not_called()


if __name__ == '__main__':
    unittest.main()
