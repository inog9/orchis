"""Behavior checks using the installed jsluice, without accessing a target."""
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
RUNNER = ROOT / 'pentest-js-analysis/scripts/analyze.py'


@unittest.skipUnless(shutil.which('jsluice'), 'jsluice is not installed')
class AnalysisTests(unittest.TestCase):
    def test_parse_errors_are_visible_even_when_commands_succeed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'source'
            source.mkdir()
            (source / 'broken.js').write_text('function broken( {')
            out = root / 'results'
            completed = subprocess.run(['python3', str(RUNNER), str(source), '--out', str(out)],
                                       capture_output=True, text=True)
            self.assertEqual(completed.returncode, 3, completed.stderr)
            manifest = json.loads((out / 'manifest.json').read_text())
            self.assertEqual(manifest['files_with_parse_errors'], 1)
            self.assertGreater(manifest['parse_error_records'], 0)

    def test_queries_compile_and_capture_expected_primitives(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'source'
            source.mkdir()
            fixture = source / 'sample with spaces.js'
            fixture.write_text('''
const q = new URLSearchParams(location.search).get("next");
const h = location.hash;
const ref = document.referrer;
window.addEventListener("message", e => { document.body.innerHTML = e.data; });
location.href = q;
eval(q);
fetch("/api/review/" + q);
el.setAttribute("href", q);
document.cookie = q;
localStorage.setItem(q, h);
q.match(/a+/);
''')
            before = fixture.read_bytes()
            metadata = source / '.state'
            metadata.mkdir()
            (metadata / 'not-target.js').write_text('eval(location.hash)')
            out = root / 'results'
            completed = subprocess.run(['python3', str(RUNNER), str(source), '--out', str(out)],
                                       capture_output=True, text=True)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            manifest = json.loads((out / 'manifest.json').read_text())
            runs = {r['mode']: r for r in manifest['files'][0]['runs']}
            self.assertEqual(len(manifest['queries']), 13)
            self.assertEqual(manifest['discovered'], 1)
            self.assertEqual(len(runs), 16)
            self.assertTrue(all(r['status'] == 0 for r in runs.values()))
            self.assertEqual(runs['parse-errors']['records'], 0)
            for name in ['source-parameter', 'source-hash', 'source-receiveMessage',
                         'source-otherSource', 'sink-write', 'sink-directExec',
                         'sink-redirect', 'sink-setAttr', 'sink-cookieStorage', 'sink-regex']:
                self.assertGreater(runs[name]['records'], 0, name)
            self.assertEqual(fixture.read_bytes(), before)
            # Reusing an evidence destination must not replace prior outputs.
            again = subprocess.run(['python3', str(RUNNER), str(source), '--out', str(out)],
                                   capture_output=True, text=True)
            self.assertNotEqual(again.returncode, 0)


if __name__ == '__main__':
    unittest.main()
