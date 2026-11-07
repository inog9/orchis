#!/usr/bin/env python3
"""Build the private engine locally; no GitHub credentials enter Docker layers."""
import hashlib, json, os, subprocess
from pathlib import Path
repo=Path(__file__).resolve().parents[2]
source=Path(os.environ.get('ORCHIS_HACKBOT_SOURCE',str(repo.parent/'orchis-hackbot-lite'))).resolve()
lock=json.loads((repo/'deploy/hackbot/engine-lock.json').read_text())
head=subprocess.check_output(['git','-C',str(source),'rev-parse','HEAD'],text=True).strip()
if head!=lock['revision']:raise SystemExit('Hackbot checkout must match the engine revision pinned in engine-lock.json.')
if not (source/'docker/export-result.mjs').is_file():raise SystemExit('The pinned Hackbot checkout is missing the service-result exporter.')
for name,digest in lock['files'].items():
    if hashlib.sha256((source/name).read_bytes()).hexdigest()!=digest:raise SystemExit('Hackbot integration file does not match engine-lock.json: '+name)
subprocess.run(['docker','build','--tag','orchis-hackbot-engine:orchis-v1',str(source)],check=True)
