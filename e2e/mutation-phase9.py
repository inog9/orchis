#!/usr/bin/env python3
"""Buktikan penjaga inti Fase 9 dengan mutasi di salinan terisolasi, bukan checkout aktif."""
from pathlib import Path
import os
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
WORK = ROOT / "work"
WORK.mkdir(exist_ok=True)
SANDBOX = Path(tempfile.mkdtemp(prefix="phase9-mutation-", dir=WORK))
for name in ("internal", "migrations"):
    shutil.copytree(ROOT / name, SANDBOX / name)
for name in ("go.mod", "go.sum"):
    shutil.copy2(ROOT / name, SANDBOX / name)

CASES = [
    (
        "authorization",
        "internal/organizations/readiness.go",
        [("if !authorized {", "if false {")],
        "./internal/server",
        "TestAssessmentScopeIdempotencyFrozenTargetsAndCancel",
    ),
    (
        "condition",
        "internal/chain/conditions.go",
        [("return a > b", "return a < b")],
        "./internal/chain",
        "TestConditionOperatorsAndMissingData",
    ),
    (
        "snapshot",
        "internal/reporting/reporting.go",
        [
            (
                "AND NOT EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id)",
                "",
            ),
            (
                "ON CONFLICT(tenant_id,assessment_id) DO NOTHING",
                "ON CONFLICT(tenant_id,assessment_id) DO UPDATE SET snapshot=EXCLUDED.snapshot",
            ),
        ],
        "./internal/server",
        "TestReportSnapshotIsFrozenRedactedAndTenantScoped",
    ),
]


def run(package, test, log_name):
    result = subprocess.run(
        ["go", "test", package, "-run", "^" + test + "$", "-count=1"],
        cwd=SANDBOX,
        env={**os.environ, "GOWORK": "off"},
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        timeout=180,
    )
    (SANDBOX / log_name).write_text(result.stdout)
    return result


for name, relative_path, replacements, package, test in CASES:
    path = SANDBOX / relative_path
    original = path.read_text()
    normal = run(package, test, name + "-before.log")
    if normal.returncode:
        raise RuntimeError(f"Baseline failed for {name}; see {SANDBOX}")
    mutated = original
    for old, new in replacements:
        if old not in mutated:
            raise RuntimeError(f"Mutation anchor is missing for {name}: {old}")
        mutated = mutated.replace(old, new, 1)
    try:
        path.write_text(mutated)
        result = run(package, test, name + "-mutated.log")
        if result.returncode == 0 or "--- FAIL: " + test not in result.stdout:
            raise RuntimeError(f"Expected test failure was not detected for {name}; see {SANDBOX}")
    finally:
        path.write_text(original)
    restored = run(package, test, name + "-restored.log")
    if restored.returncode:
        raise RuntimeError(f"Restored test failed for {name}; see {SANDBOX}")
    print(f"LULUS: mutasi {name} terdeteksi, lalu tes kembali hijau.", flush=True)
print(f"Bukti mutasi: {SANDBOX}")
