#!/usr/bin/env python3
"""Periksa isi dan izin image publik tanpa memulai proses atau memasang secret."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile
import uuid

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--image', default='orchis-server:local')
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
manifest_bytes = (repo / 'deploy/mitre/manifest.json').read_bytes()
manifest = json.loads(manifest_bytes)
bundle = 'opt/orchis/mitre/' + manifest['file']
expected_files = {
    'opt/orchis/mitre/manifest.json': manifest_bytes,
    'licenses/mitre.LICENSE': (repo / 'docs/licenses/mitre.LICENSE').read_bytes(),
}
directories = {'opt', 'opt/orchis', 'opt/orchis/mitre', 'licenses'}
seen = set()
container = None
process = None
try:
    container = subprocess.check_output([
        'docker', 'create', '--name', 'orchis-mitre-image-check-' + uuid.uuid4().hex[:12],
        '--network', 'none', '--read-only', '--cap-drop', 'ALL',
        '--security-opt', 'no-new-privileges:true', args.image,
    ], text=True).strip()
    user = subprocess.check_output(['docker', 'inspect', '--format', '{{.Config.User}}', container], text=True).strip()
    if user.split(':')[0] != '10001':
        raise RuntimeError('Server image must run as UID10001')
    process = subprocess.Popen(['docker', 'export', container], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    total = 0
    with tarfile.open(fileobj=process.stdout, mode='r|') as archive:
        for member in archive:
            total += member.size
            if total > 1 << 30:
                raise RuntimeError('Public image inspection exceeds the 1GiB budget')
            name = member.name.rstrip('/')
            if name in directories:
                if not member.isdir() or not member.mode & 0o001:
                    raise RuntimeError('Packaged directory is not traversable by UID10001: ' + name)
                seen.add(name)
            if name == bundle or name in expected_files:
                if not member.isfile() or not member.mode & 0o004:
                    raise RuntimeError('Packaged data is not a readable regular file: ' + name)
                if name.startswith('opt/orchis/mitre/') and member.mode & 0o333:
                    raise RuntimeError('Bundled JSON must be read-only and non-executable: ' + name)
                data = archive.extractfile(member)
                if name == bundle:
                    digest = hashlib.sha256()
                    length = 0
                    while chunk := data.read(65536):
                        length += len(chunk)
                        if length > manifest['bytes']:
                            raise RuntimeError('Packaged bundle exceeds its pinned size')
                        digest.update(chunk)
                    if length != manifest['bytes'] or digest.hexdigest() != manifest['sha256']:
                        raise RuntimeError('Packaged bundle does not match the committed pin')
                elif data.read(len(expected_files[name]) + 1) != expected_files[name]:
                    raise RuntimeError('Packaged manifest or license differs from the checkout: ' + name)
                seen.add(name)
    if process.wait(timeout=30) != 0:
        raise RuntimeError('Could not inspect the public image')
    if seen != directories | set(expected_files) | {bundle}:
        raise RuntimeError('Required MITRE data or license is missing from the image')
    print(json.dumps({'image': args.image, 'uid': user, 'bundle_sha256': manifest['sha256'],
                      'bytes': manifest['bytes'], 'directories_traversable': True, 'license': True}))
finally:
    if process is not None:
        if process.poll() is None:
            process.terminate()
        process.wait(timeout=30)
        process.stdout.close()
    if container:
        subprocess.run(['docker', 'rm', container], check=True, stdout=subprocess.DEVNULL)
