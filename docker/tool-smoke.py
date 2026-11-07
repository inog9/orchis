#!/usr/bin/env python3
"""Offline CLI capability smoke, plus bounded local ffuf/jsluice fixtures.

No target, registry, mailbox, authenticator token, or OAST registration is used.
Run in the image; --only NAME... selects a subset for developer validation.
"""
import argparse
import base64
import http.server
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading
import urllib.parse


COMMANDS = {
    "ffuf": [["ffuf", "-V"], ["ffuf", "-h"]],
    "httpx": [["httpx", "-duc", "-version"], ["httpx", "-duc", "-h"]],
    "gau": [["gau", "--version"], ["gau", "--help"]],
    "jsluice": [["jsluice", "--help"]],
    "totp-cli": [["totp-cli", "--version"], ["totp-cli", "generate", "--help"], ["totp-cli", "instant", "--help"]],
    "gog": [["gog", "--version"], ["gog", "--help"], ["gog", "schema", "gmail", "--json"],
            ["gog", "--readonly", "--no-input", "--wrap-untrusted", "gmail", "search", "--help"]],
    "interactsh-client": [["interactsh-client", "-duc", "-version"], ["interactsh-client", "-duc", "-h"]],
    "npxconfuse": [["npxconfuse", "--help"], ["npxconfuse", "scan", "--help"], ["npxconfuse", "check", "--help"]],
    "gori": [["gori", "--version"], ["gori", "run", "capture", "--help"], ["gori", "mcp", "--help"]],
    "interceptor": [["interceptor", "--help"]],
    "jxscout": [["jxscout", "--help"], ["jxscout", "doctor", "--format", "json"]],
    "chromium": [["chromium", "--version"]],
    "pentest-harnes": [["pentest-harnes", "--help"]],
    "pi": [["pi", "--version"]],
    "node": [["node", "--version"]],
    "npm": [["npm", "--version"]],
    "bun": [["bun", "--version"]],
    "go": [["go", "version"]],
    "prettier": [["prettier", "--version"]],
    "webcrack": [["webcrack", "--help"]],
    "python3": [["python3", "--version"]],
    "curl": [["curl", "--version"]],
    "git": [["git", "--version"]],
    "jq": [["jq", "--version"]],
    "rg": [["rg", "--version"]],
    "unzip": [["unzip", "-v"]],
    "zip": [["zip", "-v"]],
    "xz": [["xz", "--version"]],
    "file": [["file", "--version"]],
    "openssl": [["openssl", "version"]],
    "bash": [["bash", "--version"]],
    "tar": [["tar", "--version"]],
    "gzip": [["gzip", "--version"]],
    "bzip2": [["bzip2", "--help"]],
    "awk": [["awk", "--version"]],
    "sed": [["sed", "--version"]],
    "grep": [["grep", "--version"]],
    "base64": [["base64", "--version"]],
    "tr": [["tr", "--version"]],
    "sort": [["sort", "--version"]],
    "uniq": [["uniq", "--version"]],
    "wc": [["wc", "--version"]],
    "sha256sum": [["sha256sum", "--version"]],
    "find": [["find", "--version"]],
    "xargs": [["xargs", "--version"]],
    "diff": [["diff", "--version"]],
    "ps": [["ps", "--version"]],
}


def command(args, root, env, timeout=25):
    result = subprocess.run(args, cwd=root, env=env, stdin=subprocess.DEVNULL,
                            capture_output=True, timeout=timeout)
    output = result.stdout + result.stderr
    if result.returncode:
        raise RuntimeError(f"{args!r} exited {result.returncode}: {output.decode(errors='replace')[-1500:]}")
    return result.stdout, output


def ffuf_fixture(root, env):
    class Handler(http.server.BaseHTTPRequestHandler):
        paths = []
        agents = []

        def do_GET(self):
            type(self).paths.append(self.path)
            type(self).agents.append(self.headers.get("User-Agent"))
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"local-only ffuf fixture")

        def log_message(self, *_args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    words = ["alpha", "percent%2Fencoded"]
    wordlist = root / "ffuf-words.txt"
    wordlist.write_text("\n".join(words) + "\n")
    try:
        stdout, _ = command(["ffuf", "-w", str(wordlist), "-u", f"http://127.0.0.1:{server.server_port}/FUZZ",
                             "-json", "-raw", "-noninteractive", "-t", "1", "-rate", "2", "-maxtime", "5",
                             "-timeout", "2", "-mc", "all", "-H", "User-Agent: orchis-tool-smoke", "-s"], root, env)
        rows = [json.loads(line) for line in stdout.splitlines() if line.strip()]
        decoded = {base64.b64decode(row["input"]["FUZZ"], validate=True).decode() for row in rows}
        if decoded != set(words) or len(rows) != 2:
            raise RuntimeError(f"ffuf JSONL input.FUZZ contract changed: {rows!r}")
        paths = {urllib.parse.urlsplit(path).path if path.startswith("http://") else path for path in Handler.paths}
        if paths != {"/" + word for word in words}:
            raise RuntimeError(f"ffuf -raw changed encoded request paths: {Handler.paths!r}")
        if Handler.agents != ["orchis-tool-smoke"] * 2:
            raise RuntimeError(f"ffuf header contract changed: {Handler.agents!r}")
    finally:
        server.shutdown()
        server.server_close()


def jsluice_fixture(root, env):
    source = root / "tool-smoke.js"
    source.write_text('fetch("/tool-smoke-url");\n')
    stdout, _ = command(["jsluice", "urls", "-S", str(source)], root, env)
    rows = [json.loads(line) for line in stdout.splitlines() if line.strip()]
    if not any(row.get("url") == "/tool-smoke-url" for row in rows):
        raise RuntimeError("jsluice tree-sitter grammar did not extract the local fixture URL")
    command(["jsluice", "secrets", str(source)], root, env)
    command(["jsluice", "query", "-f", "-r", "-q", "(call_expression) @call", str(source)], root, env)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--only", nargs="+", choices=sorted(COMMANDS))
    names = parser.parse_args().only or list(COMMANDS)
    failed = []
    with tempfile.TemporaryDirectory(prefix="pentest-tools-smoke-") as temp:
        root = Path(temp)
        env = {key: value for key, value in os.environ.items()
               if key.lower() not in {"http_proxy", "https_proxy", "all_proxy", "no_proxy"}}
        env.update(HOME=temp, XDG_CONFIG_HOME=str(root / "config"), PI_CODING_AGENT_DIR=str(root / "pi"),
                   INTERCEPTOR_TEMP=str(root / "interceptor"), GORI_HOME=str(root / "gori"),
                   GOTOOLCHAIN="local", NO_COLOR="1")
        for name in names:
            try:
                if not shutil.which(name, path=env.get("PATH")):
                    raise RuntimeError("executable is missing from PATH")
                outputs = [command(args, root, env)[1] for args in COMMANDS[name]]
                if name == "httpx" and not all(flag in b"\n".join(outputs) for flag in (b"-tech-detect", b"-status-code")):
                    raise RuntimeError("httpx must be ProjectDiscovery's CLI, not Python httpx")
                if name == "gog" and not all(flag in outputs[1] for flag in (b"--readonly", b"--wrap-untrusted", b"--no-input")):
                    raise RuntimeError("gog is missing the bundled Gmail skill's safety flags")
                if name == "ffuf":
                    ffuf_fixture(root, env)
                if name == "jsluice":
                    jsluice_fixture(root, env)
                print(f"PASS {name}", flush=True)
            except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as exc:
                failed.append(name)
                print(f"FAIL {name}: {exc}", flush=True)
    if failed:
        raise SystemExit("Tool smoke failed: " + ", ".join(failed))


if __name__ == "__main__":
    main()
