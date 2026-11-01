#!/usr/bin/env python3
"""Mutasi pemilihan agent hanya pada salinan sumber disposable."""
from pathlib import Path
import json
import shutil
import subprocess
import time

source = Path(__file__).resolve().parents[1]
out = source / "work" / ("agent-selection-mutations-" + str(time.time_ns()))
repo = out / "src"
repo.mkdir(parents=True)
for name in ("internal", "migrations"):
    shutil.copytree(source / name, repo / name)
for name in ("go.mod", "go.sum"):
    shutil.copy2(source / name, repo / name)

def run(label):
    result = subprocess.run(
        ["go", "test", "./internal/demo", "-run", "^TestDemoAgentsCannotReceiveNewEndpointWork$", "-count=1"],
        cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90,
    )
    (out / (label + ".log")).write_text(result.stdout)
    return result

assert run("baseline").returncode == 0, "baseline failed"
cases = [
    ("inventory", "internal/endpoint/endpoint.go", "AND NOT ag.is_demo", "advertised an executable agent"),
    ("atomic", "internal/execution/service.go", "AND NOT is_demo", "accepted an atomic request"),
    ("launch", "internal/simulation/service.go", "AND asset_id=$2 AND NOT is_demo", "accepted scenario launch"),
    ("follow-up", "internal/simulation/engine.go", "AND asset_id=$2 AND NOT is_demo", "received a chain follow-up"),
]
results = []
for label, name, old, message in cases:
    file = repo / name
    original = file.read_text()
    assert original.count(old) == 1, (label, original.count(old))
    try:
        file.write_text(original.replace(old, old.replace("AND NOT ag.is_demo", "").replace("AND NOT is_demo", "")))
        result = run(label)
        assert result.returncode != 0 and message in result.stdout, "mutation survived or failed for another reason: " + label
        results.append({"name": label, "killed": True})
        print(label + ": killed", flush=True)
    finally:
        file.write_text(original)
assert run("restored").returncode == 0, "restored source failed"
(out / "results.json").write_text(json.dumps(results, indent=2))
print("Restored source passed; evidence " + str(out), flush=True)
