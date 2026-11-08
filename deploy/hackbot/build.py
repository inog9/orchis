#!/usr/bin/env python3
"""Validate both pinned engine checkouts before building local Docker images."""
import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]


class SourceError(ValueError):
    """A missing or incompatible engine checkout."""


def check_source(source, lock, label):
    if not source.is_dir():
        raise SourceError(
            f'{label} source not found: {source}\n'
            f'Clone {lock["repository"]} there and check out {lock["revision"]}.')
    try:
        head = subprocess.check_output(
            ['git', '-C', str(source), 'rev-parse', 'HEAD'],
            text=True, stderr=subprocess.PIPE).strip()
    except subprocess.CalledProcessError as exc:
        raise SourceError(
            f'{label} source must be a Git checkout: {source}\n'
            f'Use {lock["repository"]} at {lock["revision"]}; '
            'an extracted source archive has no verifiable commit.') from exc
    if head != lock['revision']:
        raise SourceError(
            f'{label} revision mismatch in {source}: {head}\n'
            f'Required revision: {lock["revision"]}.')
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
        ('Hackbot Lite', 'ORCHIS_HACKBOT_SOURCE', 'orchis-hackbot-lite', lock),
        ('OpenCode', 'ORCHIS_OPENCODE_SOURCE', 'orchis-opencode-hackbot', lock['opencode']),
    ]
    checked = []
    for label, variable, directory, pin in sources:
        source = Path(os.environ.get(variable, str(repo.parent / directory))).resolve()
        checked.append(check_source(source, pin, label))
    return checked


def build_images(sources):
    for source, image in zip(sources, ['orchis-hackbot-engine:orchis-v1', 'orchis-opencode-engine:orchis-v1']):
        subprocess.run(['docker', 'build', '--tag', image, str(source)], check=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='validate sources without running Docker')
    args = parser.parse_args(argv)
    try:
        sources = check_sources()
    except SourceError as exc:
        parser.exit(1, f'{exc}\nSee deploy/hackbot/README.md for engine setup.\n')
    print('Both engine revisions and integration hashes match engine-lock.json.', flush=True)
    if not args.check:
        build_images(sources)


if __name__ == '__main__':
    main()
