#!/usr/bin/env python3
"""Mutasi pemisahan bukti MITRE dan deduplikasi hierarki pada salinan sumber."""
from pathlib import Path
import json
import shutil
import subprocess
import time

source = Path(__file__).resolve().parents[1]
out = source / 'work' / ('mitre-coverage-mutations-' + str(time.time_ns()))
repo = out / 'src'
repo.mkdir(parents=True)
for name in ['internal', 'migrations']:
    shutil.copytree(source / name, repo / name)
for name in ['go.mod', 'go.sum']:
    shutil.copy2(source / name, repo / name)
test = '^TestMITRECoverageKeepsOverlappingEvidenceSeparateAndDeduplicated$'

def run(label):
    result = subprocess.run(['go', 'test', './internal/demo', '-run', test, '-count=1'], cwd=repo,
                            text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
    (out / (label + '.log')).write_text(result.stdout)
    return result

assert run('baseline').returncode == 0, 'baseline failed'
cases = [
    ('real-filter', "WHERE e.tenant_id=$1 AND NOT e.is_demo AND e.state='ended'", "WHERE e.tenant_id=$1 AND e.state='ended'", 'incorrect real training count'),
    ('demo-run-filter', "WHERE e.tenant_id=$1 AND e.is_demo AND e.state='ended'", "WHERE e.tenant_id=$1 AND e.state='ended'", 'evidence conflated or duplicated'),
    ('demo-finding-filter', 'WHERE f.tenant_id=$1 AND f.is_demo', 'WHERE f.tenant_id=$1', 'parent lost child demo finding'),
    ('parent-dedup', 'count(DISTINCT e.id)', 'count(*)', 'evidence conflated or duplicated'),
    ('overlap', 'c.Simulated = st.SimulatedRuns > 0 || st.SimulatedFindings > 0', 'c.Simulated = (st.SimulatedRuns > 0 || st.SimulatedFindings > 0) && !c.Mapped', 'mapped and simulated must overlap'),
    ('failed-run-filter', "WHERE e.tenant_id=$1 AND e.is_demo AND e.state='ended' AND e.outcome='success'", "WHERE e.tenant_id=$1 AND e.is_demo AND e.state='ended'", 'evidence conflated or duplicated'),
]
path = repo / 'internal/mitre/coverage.go'
original = path.read_text()
results = []
for label, old, new, expected in cases:
    assert original.count(old) == 1, (label, original.count(old))
    try:
        path.write_text(original.replace(old, new))
        result = run(label)
        assert result.returncode != 0 and expected in result.stdout, 'mutation survived or failed for another reason: ' + label
        print(label + ': killed', flush=True)
        results.append({'name': label, 'killed': True})
    finally:
        path.write_text(original)
assert run('restored').returncode == 0, 'restored source failed'
(out / 'results.json').write_text(json.dumps(results, indent=2))
print('Restored source passed; evidence ' + str(out), flush=True)
