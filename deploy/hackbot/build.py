#!/usr/bin/env python3
"""Validate and build the Hackbot tool and OpenCode engines from this repository."""
import argparse
import hashlib
import json
import subprocess
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]


class SourceError(ValueError):
    """A missing or incompatible bundled engine source."""


def check_source(source, lock, label):
    if not source.is_dir():
        raise SourceError(f'{label} bundled source not found: {source}')
    for name, digest in lock['files'].items():
        path = source / name
        if not path.is_file():
            raise SourceError(f'{label} integration file is missing: {path}')
        if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise SourceError(f'{label} integration file differs from engine-lock.json: {path}')
    return source


def check_sources(repo=REPO):
    lock = json.loads((repo / 'deploy/hackbot/engine-lock.json').read_text())
    sources = [
        ('Hackbot Lite', repo / 'engines/hackbot-lite', lock),
        ('OpenCode', repo / 'engines/opencode-hackbot', lock['opencode']),
    ]
    return [check_source(path, pin, label) for label, path, pin in sources]


def build_images(sources):
    for source, image in zip(sources, ['orchis-hackbot-engine:orchis-v1', 'orchis-opencode-engine:orchis-v1']):
        subprocess.run(['docker', 'build', '--tag', image, str(source)], check=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='validate bundled sources without running Docker')
    args = parser.parse_args(argv)
    try:
        sources = check_sources()
    except SourceError as exc:
        parser.exit(1, f'{exc}\nSee deploy/hackbot/README.md for engine setup.\n')
    print('Bundled engine integration hashes match engine-lock.json.', flush=True)
    if not args.check:
        build_images(sources)


if __name__ == '__main__':
    main()
