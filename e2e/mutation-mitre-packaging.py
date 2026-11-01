#!/usr/bin/env python3
"""Uji pin dan batas unduhan MITRE pada salinan sumber, dengan fixture TLS lokal."""
from pathlib import Path
import json
import shutil
import subprocess
import time

source = Path(__file__).resolve().parents[1]
out = source / 'work' / ('mitre-packaging-mutations-' + str(time.time_ns()))
repo = out / 'src'
repo.mkdir(parents=True)
for name in ['internal', 'migrations']:
    shutil.copytree(source / name, repo / name)
for name in ['go.mod', 'go.sum']:
    shutil.copy2(source / name, repo / name)

tests = '^Test(FetchUsesCommittedPinAndInstallsAtomically|ManifestRejectsUnsafeMetadataBeforeNetwork|ProductionRequiresPinnedMitreBundle)$'

def run(label, package=None, test=tests):
    command = ['go', 'test', *( [package] if package else ['./internal/mitre', './internal/config'] ), '-run', test, '-count=1']
    result = subprocess.run(command, cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
    (out / (label + '.log')).write_text(result.stdout)
    return result

assert run('baseline').returncode == 0, 'baseline failed'
cases = [
    ('hash', 'internal/mitre/fetch.go', 'ParseVerified(f, m.SHA256)', 'ParseVerified(f, "")', './internal/mitre', '^TestFetchUsesCommittedPinAndInstallsAtomically/hash$', 'hash pin was ignored'),
    ('size', 'internal/mitre/freeze.go', '!info.Mode().IsRegular() || info.Size() != m.Bytes', '!info.Mode().IsRegular()', './internal/mitre', '^TestFetchUsesCommittedPinAndInstallsAtomically/#00$', 'manifest size mismatch was ignored'),
    ('metadata', 'internal/mitre/freeze.go', 'if err := m.validate(); err != nil {\n\t\treturn nil, err\n\t}', '', './internal/mitre', '^TestManifestRejectsUnsafeMetadataBeforeNetwork$', 'unsafe metadata reached network or was accepted'),
    ('runtime-url', 'internal/config/config.go', '!filepath.IsAbs(c.MitreSource) || ', '(!filepath.IsAbs(c.MitreSource) && false) || ', './internal/config', '^TestProductionRequiresPinnedMitreBundle$', 'production accepted runtime network source'),
    ('cached-network', 'internal/mitre/fetch.go', 'if _, ds, err := Verify(dir); err == nil {', 'if _, ds, err := Verify(dir); err == nil && false {', './internal/mitre', '^TestFetchUsesCommittedPinAndInstallsAtomically/#00$', 'valid cache triggered another network request'),
]
results = []
for label, name, old, new, package, test, expected in cases:
    path = repo / name
    original = path.read_text()
    assert original.count(old) == 1, (label, original.count(old))
    try:
        path.write_text(original.replace(old, new))
        result = run(label, package, test)
        assert result.returncode != 0 and expected in result.stdout, 'mutation survived or failed for another reason: ' + label
        print(label + ': killed', flush=True)
        results.append({'name': label, 'killed': True})
    finally:
        path.write_text(original)
assert run('restored').returncode == 0, 'restored source failed'
(out / 'results.json').write_text(json.dumps(results, indent=2))
print('Restored source passed; evidence ' + str(out), flush=True)
