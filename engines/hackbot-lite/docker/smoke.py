#!/usr/bin/env python3
"""Local-only integration smoke: Interceptor -> HTTPS -> Gori -> JXScout.

Run inside the built container: python3 /opt/pentest/docker/smoke.py
Uses an ephemeral loopback HTTPS fixture and no model credentials or target host.
"""
import functools
import http.server
import json
import os
from pathlib import Path
import signal
import ssl
import subprocess
import tempfile
import threading
import time


def command(args, **kw):
    return subprocess.run(args, check=True, timeout=35, capture_output=True, **kw)


def wait_for(question, timeout=60):
    until = time.monotonic() + timeout
    last = None
    while time.monotonic() < until:
        try:
            value = question()
            if value:
                return value
        except (OSError, subprocess.SubprocessError, ValueError) as exc:
            last = exc
        time.sleep(.5)
    raise RuntimeError(f"timed out waiting for {question.__name__}: {last}")


def main():
    with tempfile.TemporaryDirectory(prefix="pentest-stack-smoke-") as temp:
        root = Path(temp)
        web = root / "fixture"
        web.mkdir()
        (web / "index.html").write_text('<!doctype html><title>Stack fixture</title><h1>Gori bridge fixture</h1><script src="/app.js"></script>')
        # The browser never calls this function: only JXScout discovers lazy.js.
        (web / "app.js").write_text('window.loadFixtureChunk = () => import("./lazy.js");\n')
        (web / "lazy.js").write_text('export const fixture = "chunk-discovered-through-gori";\n')
        command(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=Fixture CA",
                 "-keyout", str(root / "ca.key"), "-out", str(root / "ca.pem")])
        command(["openssl", "req", "-newkey", "rsa:2048", "-nodes", "-subj", "/CN=127.0.0.1",
                 "-keyout", str(root / "server.key"), "-out", str(root / "server.csr")])
        (root / "extensions.cnf").write_text("subjectAltName=IP:127.0.0.1\nextendedKeyUsage=serverAuth\n")
        command(["openssl", "x509", "-req", "-days", "1", "-in", str(root / "server.csr"), "-CA", str(root / "ca.pem"),
                 "-CAkey", str(root / "ca.key"), "-CAcreateserial", "-extfile", str(root / "extensions.cnf"), "-out", str(root / "server.pem")])
        handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(web))
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(root / "server.pem", root / "server.key")
        server.socket = tls.wrap_socket(server.socket, server_side=True)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        origin = f"https://127.0.0.1:{server.server_port}"
        session = root / "session"
        session.mkdir()
        (session / "state.json").write_text(json.dumps({"mode": "running", "scope": {"origins": [origin], "methods": ["GET"]}}))
        env = dict(os.environ, SSL_CERT_FILE=str(root / "ca.pem"), INTERCEPTOR_TEMP=str(session / "private/stack/interceptor"),
                   GORI_HOME=str(session / "private/stack/gori"))
        stack_log = open(root / "stack.log", "wb")
        supervisor = subprocess.Popen(["python3", "/opt/pentest/docker/stack.py", str(session)], env=env, stdout=stack_log, stderr=stack_log)
        try:
            def ready():
                if supervisor.poll() is not None:
                    raise RuntimeError((root / "stack.log").read_text())
                return (session / "private/stack/ready.json").exists()
            wait_for(ready, timeout=85)
            command(["interceptor", "open", origin + "/index.html", "--group", "stack-smoke"], env=env)
            db = session / "private/stack/gori.db"
            def captured_chunk():
                data = command(["gori", "run", "history", "--db", str(db), "--format", "json"], env=env).stdout
                return b"/lazy.js" in data
            wait_for(captured_chunk)
            project = session / "private/stack/jxscout-workspace"
            def indexed_chunk():
                data = command(["jxscout", "assets", "--project", session.name, "--format", "json"], cwd=project,
                               env=dict(env, JXSCOUT_DATA_DIR=str(session / "private/stack/jxscout-data"))).stdout
                return b"lazy.js" in data
            wait_for(indexed_chunk)
            print("PASS: headless Interceptor + trusted HTTPS + Gori capture + JXScout ingestion + discovered chunk proxied through Gori")
        except Exception:
            stack_log.flush()
            print((root / "stack.log").read_text(errors="replace"))
            for log in (session / "private/stack").glob("*.log"):
                print(f"--- {log.name} ---\n{log.read_text(errors='replace')[-10000:]}")
            raise
        finally:
            supervisor.send_signal(signal.SIGTERM)
            try:
                supervisor.wait(timeout=50)
            except subprocess.TimeoutExpired:
                supervisor.kill()
                supervisor.wait()
            stack_log.close()
            server.shutdown()


if __name__ == "__main__":
    main()
