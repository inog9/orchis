"""Pin MITRE untuk satu deployment; tidak menyentuh kredensial atau service yang berjalan."""
import json
import os
import re
import secrets
import time


def manifest(repo):
    value = json.loads((repo / 'deploy/mitre/manifest.json').read_text())
    version = value.get('version', '')
    if (value.get('matrix') != 'enterprise'
            or not re.fullmatch(r'[0-9]+\.[0-9]+(?:\.[0-9]+)?', version)
            or value.get('file') != 'enterprise-attack-' + version + '.json'
            or not re.fullmatch(r'[a-fA-F0-9]{64}', value.get('sha256', ''))):
        raise RuntimeError('Invalid frozen ATT&CK manifest')
    return value


def defaults(repo):
    value = manifest(repo)
    return {'ORCHIS_MITRE_URL': '/opt/orchis/mitre/' + value['file'],
            'ORCHIS_MITRE_SHA256': value['sha256'].lower()}


def configure(args, repo, private):
    if os.geteuid() != 0:
        raise RuntimeError('MITRE configuration requires the root operator')
    pin = defaults(repo)
    changes = []
    for name in ['server.json', 'migrate.json', 'provision.json']:
        path = args.state / name
        if path.is_symlink() or not path.is_file() or path.stat().st_mode & 0o077 or path.stat().st_size > 65536:
            raise RuntimeError('Application configuration must be a private regular file')
        value = json.loads(path.read_text())
        value.update(pin)
        changes.append((path, value))
    for path, value in changes:
        temporary = args.state / (path.name + '.mitre-' + secrets.token_hex(4))
        try:
            private(temporary, value)
            os.replace(temporary, path)
        finally:
            temporary.unlink(missing_ok=True)
    print('Pinned local ATT&CK configuration updated; build the matching image and restart the selected server to apply it.')


def verify_import(client, base, repo):
    expected = manifest(repo)
    counts = expected['counts']
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        with client.open(base + '/api/v1/mitre/matrix', timeout=10) as response:
            matrix = json.load(response)
        run = matrix.get('last_run') or {}
        if run.get('status') == 'failed':
            raise RuntimeError('ATT&CK import failed; inspect the selected server log')
        if matrix.get('imported') and run.get('status') == 'succeeded':
            parents = {c['id']: c for column in matrix['tactics'] for c in column['techniques']}
            subs = {s['id'] for c in parents.values() for s in c.get('subs', [])}
            if (run.get('source_sha256') != expected['sha256']
                    or run.get('source_version') != expected['version']
                    or matrix.get('total') != counts['techniques']
                    or len(parents) != counts['techniques']
                    or len(subs) != counts['subtechniques']
                    or len(matrix['tactics']) != counts['tactics']):
                raise RuntimeError('Loaded ATT&CK data does not match the frozen manifest')
            print('Local ATT&CK bundle verified: version ' + expected['version'] + ', complete hierarchy and pinned SHA-256')
            return
        time.sleep(0.25)
    raise RuntimeError('Timed out waiting for the local ATT&CK import')
