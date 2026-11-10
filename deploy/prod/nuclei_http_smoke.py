#!/usr/bin/env python3
"""Exercise HTTP-only filtering and Nuclei against a local fixture, not a real target."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from filter_http_templates import filter_templates


class Fixture(BaseHTTPRequestHandler):
    paths = []

    def do_GET(self):
        self.paths.append(self.path)
        self.send_response(200 if self.path == '/orchis-http-probe' else 404)
        self.end_headers()
        self.wfile.write(b'orchis fixture')

    def log_message(self, *_args):
        pass


def main():
    server = ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix='orchis-http-fixture-') as temp:
            root = Path(temp)
            os.chmod(root, 0o755)
            header = 'info:\n  name: local fixture\n  author: orchis\n  severity: info\n'
            http = 'http:\n  - method: GET\n    path:\n      - "{{BaseURL}}/orchis-http-probe"\n    matchers:\n      - type: status\n        status: [200]\n'
            (root / 'http.yaml').write_text('id: orchis-http-fixture\n' + header + http)
            (root / 'mixed.yaml').write_text('id: orchis-mixed-fixture\n' + header + http + 'tcp:\n  - inputs:\n      - data: "hello"\n    host:\n      - "{{Hostname}}"\n')
            assert filter_templates(root) == (1, 1)
            result = subprocess.run([
                'docker', 'run', '--rm', '--network', 'host', '--entrypoint', 'nuclei',
                '-v', f'{root}:/fixture:ro', '-e', 'HOME=/tmp',
                'orchis-injector-prod:http-only', '-u', f'http://127.0.0.1:{server.server_port}',
                '-t', '/fixture', '-pt', 'http', '-ni', '-duc', '-jsonl', '-silent', '-nc',
            ], capture_output=True, text=True, timeout=60, check=True)
            findings = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
            assert [f['template-id'] for f in findings] == ['orchis-http-fixture'], findings
            assert Fixture.paths and all(p == '/orchis-http-probe' for p in Fixture.paths), Fixture.paths
            print('HTTP-only Nuclei fixture passed; mixed template excluded.')
    finally:
        server.shutdown()
        server.server_close()


if __name__ == '__main__':
    main()
