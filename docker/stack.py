#!/usr/bin/env python3
"""Session-owned container services. Never runs target requests before scope."""
import datetime
import fcntl
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import sys
import time


class Stack:
    def __init__(self, session):
        self.session = Path(session).resolve()
        self.root = self.session / "private/stack"
        self.root.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.env = os.environ.copy()
        self.env.update(GORI_HOME=str(self.root / "gori"), INTERCEPTOR_TEMP=str(self.root / "interceptor"))
        (self.root / "interceptor").mkdir(exist_ok=True, mode=0o700)
        self.children = {}
        self.handles = []
        self.stopping = False
        self.scope_key = None
        self.parent_pid = os.getppid()
        self.owns_lock = False

    def run(self, args, env=None, **kwargs):
        if self.stopping:
            raise InterruptedError("stack is stopping")
        return subprocess.run(args, env=env or self.env, check=True, timeout=25, **kwargs)

    def start(self, name, args, env=None, stdout=None, cwd=None):
        if self.stopping:
            raise InterruptedError("stack is stopping")
        log = open(self.root / (name + ".log"), "ab", buffering=0)
        self.handles.append(log)
        child = subprocess.Popen(args, env=env or self.env, cwd=cwd,
                                 stdin=subprocess.DEVNULL, stdout=stdout or log,
                                 stderr=log, start_new_session=True)
        self.children[name] = child
        return child

    def stop(self, name):
        child = self.children.pop(name, None)
        if child is None:
            return
        try:
            os.killpg(child.pid, signal.SIGTERM)
        except ProcessLookupError:
            return
        try:
            child.wait(timeout=4)
        except subprocess.TimeoutExpired:
            pass
        finally:
            # Reap the whole group even if its direct leader already exited.
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            child.wait()

    def wait_port(self, port, deadline=25):
        until = time.monotonic() + deadline
        while time.monotonic() < until and not self.stopping:
            self.check_children()
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=.5):
                    return
            except OSError:
                time.sleep(.2)
        raise RuntimeError(f"service did not bind port {port}; inspect {self.root}")

    def check_children(self):
        if os.getppid() != self.parent_pid:
            raise RuntimeError("launcher exited; stopping engagement services")
        for name, child in self.children.items():
            if child.poll() is not None:
                raise RuntimeError(f"{name} exited with {child.returncode}; inspect {self.root / (name + '.log')}")

    def setup(self):
        # Ports and per-user Interceptor transport are shared inside one container.
        self.lock = open("/tmp/pentest-stack.lock", "a")
        try:
            fcntl.flock(self.lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise RuntimeError("another engagement stack is active in this container") from exc
        self.owns_lock = True
        ca = self.run(["gori", "ca", "--pem"], capture_output=True).stdout
        (self.root / "ca.pem").write_bytes(ca)
        (self.root / "trust.pem").write_bytes(Path("/etc/ssl/certs/ca-certificates.crt").read_bytes() + b"\n" + ca)
        captures = open(self.root / "capture.jsonl", "ab", buffering=0)
        self.handles.append(captures)
        self.start("gori", ["gori", "run", "capture", "--db", str(self.root / "gori.db"),
                            "--listen", "127.0.0.1", "--port", "8070", "--format", "jsonl"], stdout=captures)
        self.wait_port(8070)
        browser_home = self.root / "browser-home"
        nss = browser_home / ".pki/nssdb"
        nss.mkdir(parents=True, exist_ok=True, mode=0o700)
        if not (nss / "cert9.db").exists():
            self.run(["certutil", "-N", "--empty-password", "-d", "sql:" + str(nss)])
        # A fixed nickname makes CA trust setup idempotent after restarts.
        subprocess.run(["certutil", "-D", "-d", "sql:" + str(nss), "-n", "Gori engagement CA"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.run(["certutil", "-A", "-d", "sql:" + str(nss), "-n", "Gori engagement CA", "-t", "C,,", "-i", str(self.root / "ca.pem")])
        hosts = browser_home / ".config/chromium/NativeMessagingHosts"
        hosts.mkdir(parents=True, exist_ok=True)
        manifest = json.loads(Path("/opt/interceptor/daemon/com.interceptor.host.json").read_text())
        manifest["path"] = "/opt/interceptor/daemon/interceptor-daemon"
        (hosts / "com.interceptor.host.json").write_text(json.dumps(manifest))
        profile = self.root / "chromium"
        (profile / "Default").mkdir(parents=True, exist_ok=True)
        preferences = profile / "Default/Preferences"
        settings = json.loads(preferences.read_text()) if preferences.exists() else {}
        settings.setdefault("extensions", {}).setdefault("ui", {})["developer_mode"] = True
        preferences.write_text(json.dumps(settings))
        env = dict(self.env, HOME=str(browser_home), XDG_CONFIG_HOME=str(browser_home / ".config"))
        self.start("chromium", ["chromium", "--headless=new", "--no-sandbox", "--disable-dev-shm-usage",
                               "--no-first-run", "--no-default-browser-check", "--disable-background-networking",
                               "--disable-quic", "--proxy-server=http://127.0.0.1:8070", "--proxy-bypass-list=<-loopback>",
                               "--user-data-dir=" + str(profile), "--load-extension=/opt/interceptor/extension/dist",
                               "about:blank"], env=env)
        until = time.monotonic() + 40
        while time.monotonic() < until and not self.stopping:
            self.check_children()
            status = subprocess.run(["interceptor", "status", "--verbose"], env=self.env, capture_output=True, timeout=8)
            report = status.stdout.decode(errors="replace")
            if re.search(r"^extension:\s+reachable(?:\s|$)", report, re.MULTILINE):
                (self.root / "ready.json").write_text(json.dumps({"ready": True, "database": str(self.root / "gori.db")}))
                return
            time.sleep(.5)
        raise RuntimeError("Interceptor extension is not reachable in headless Chromium; inspect chromium.log. No virtual-display fallback is enabled.")

    def scope(self):
        try:
            state = json.loads((self.session / "state.json").read_text())
        except FileNotFoundError:
            return None
        scope = state.get("scope") or {}
        if state.get("mode") != "running" or not scope.get("origins") or "GET" not in scope.get("methods", []):
            return None
        expires = scope.get("expiresAt")
        if expires and datetime.datetime.now(datetime.timezone.utc) >= datetime.datetime.fromisoformat(expires.replace("Z", "+00:00")):
            return None
        return tuple(sorted(scope["origins"]))

    def update_collection(self):
        origins = self.scope()
        if origins == self.scope_key:
            return
        self.stop("bridge")
        self.stop("jxscout")
        self.scope_key = origins
        if not origins:
            return
        project = self.root / "jxscout-workspace"
        project.mkdir(exist_ok=True)
        history_limit = int(self.env.get("PENTEST_HARNES_HISTORY_LIMIT", "100000"))
        if history_limit < 1:
            raise ValueError("PENTEST_HARNES_HISTORY_LIMIT must be a positive integer")
        env = dict(self.env, HTTP_PROXY="http://127.0.0.1:8070", HTTPS_PROXY="http://127.0.0.1:8070",
                   JXSCOUT_PROXY_URL="http://127.0.0.1:8070",
                   NO_PROXY="", no_proxy="", SSL_CERT_FILE=str(self.root / "trust.pem"),
                   JXSCOUT_DATA_DIR=str(self.root / "jxscout-data"))
        scopes = [argument for origin in origins for argument in ("--scope", origin.rstrip("/") + "/*")]
        self.run(["jxscout", "init", "--project", self.session.name, "--force", *scopes], env=env, cwd=project,
                 stdout=subprocess.DEVNULL)
        self.start("jxscout", ["jxscout", "serve", "--headless", "--project", self.session.name,
                               "--hostname", "127.0.0.1", "--port", "3333", *scopes], env=env, cwd=project)
        self.wait_port(3333)
        self.start("bridge", ["gori-jxscout", "--db", str(self.root / "gori.db"), "--events", str(self.root / "capture.jsonl"),
                              "--state", str(self.root / "bridge-checkpoint.json"), "--scope-file", str(self.session / "state.json"),
                              "--ingest-url", "http://127.0.0.1:3333/caido-ingest", "--history-limit", str(history_limit)])

    def close(self):
        if self.owns_lock:
            (self.root / "ready.json").unlink(missing_ok=True)
        for name in list(reversed(self.children)):
            self.stop(name)
        # This daemon belongs exclusively to this container engagement.
        if self.owns_lock:
            subprocess.run(["interceptor", "daemon", "stop"], env=self.env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5)
        for handle in self.handles:
            handle.close()


def main():
    os.umask(0o077)
    stack = Stack(sys.argv[1])
    def terminate(_signum, _frame):
        stack.stopping = True
    signal.signal(signal.SIGTERM, terminate)
    signal.signal(signal.SIGINT, terminate)
    try:
        stack.setup()
        while not stack.stopping:
            stack.check_children()
            stack.update_collection()
            time.sleep(.5)
    finally:
        stack.close()


if __name__ == "__main__":
    main()
