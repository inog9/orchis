"""WordPress helper contracts: offline preparation and local-only supervision.

Fake ffuf processes make no requests. The optional installed-ffuf test sends at
most three requests, exclusively to an ephemeral loopback HTTP server.
"""
from datetime import datetime, timedelta, timezone
import base64
from contextlib import redirect_stderr
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import io
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest import mock
from urllib.parse import quote, urlsplit


ROOT = Path(__file__).resolve().parents[1]
SKILL = ROOT / "skills/wordpress-pentest"
PREPARE = SKILL / "scripts/prepare-wordlists.py"
GUARD = SKILL / "scripts/ffuf-guard.py"


def invoke(script, *args, cwd=None):
    return subprocess.run([sys.executable, str(script), *map(str, args)], cwd=cwd,
                          capture_output=True, text=True, timeout=20)


class PreparationTests(unittest.TestCase):
    def test_bundled_lists_are_lossless_and_defaults_are_script_relative(self):
        paths = [SKILL / "reference" / name for name in
                 ("wordpress-top-10000-plugins.txt", "wordpress-all-plugins.txt")]
        hashes = [hashlib.sha256(p.read_bytes()).hexdigest() for p in paths]
        top, all_slugs = [p.read_text(encoding="utf-8").splitlines() for p in paths]
        self.assertEqual((len(top), len(set(top))), (10000, 10000))
        self.assertEqual((len(all_slugs), len(set(all_slugs))), (132211, 132211))
        self.assertTrue(set(top) <= set(all_slugs))
        self.assertIn("de_de", top)
        self.assertEqual(sum("_" in s for s in all_slugs), 7)
        self.assertEqual(sum(not s.isascii() for s in all_slugs), 69)
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory) / "prepared"
            run = invoke(PREPARE, "--out", out, cwd=directory)
            self.assertEqual(run.returncode, 0, run.stderr + (out / "rejects.json").read_text())
            manifest = json.loads((out / "manifest.json").read_text())
            self.assertEqual(manifest["rejected_rows"], 0)
            self.assertEqual(manifest["outputs"]["top"]["count"], 10000)
            self.assertEqual(manifest["outputs"]["delta"]["count"], 122211)
            self.assertEqual(manifest["coverage"]["network_requests"], 0)
            top_set = set(top)
            expected = [quote(s, safe="-._~") for s in all_slugs if s not in top_set]
            self.assertEqual((out / "delta.encoded.txt").read_text().splitlines(), expected)
            self.assertEqual(len((out / "slug-map.jsonl").read_text().splitlines()), 142211)
            before = (out / "manifest.json").read_bytes()
            self.assertNotEqual(invoke(PREPARE, "--out", out).returncode, 0)
            self.assertEqual((out / "manifest.json").read_bytes(), before)
        self.assertEqual([hashlib.sha256(p.read_bytes()).hexdigest() for p in paths], hashes)

    def test_unicode_dedup_mapping_exclusions_and_reject_accounting(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            inputs = {"top": "de_de\ncafé\na\na\n", "all": "a\ncafé\n插件\nz\n",
                      "observed": "z\nobserved-new\ncafé\n", "exclude": "a\n"}
            args = []
            for label, text in inputs.items():
                path = root / (label + ".txt")
                path.write_text(text, encoding="utf-8")
                args += ["--" + label, path]
            out = root / "output"
            result = invoke(PREPARE, *args, "--out", out)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((out / "top.encoded.txt").read_text().splitlines(),
                             ["z", "observed-new", "caf%C3%A9", "de_de"])
            self.assertEqual((out / "delta.encoded.txt").read_text().splitlines(), ["%E6%8F%92%E4%BB%B6"])
            rows = [json.loads(s) for s in (out / "slug-map.jsonl").read_text().splitlines()]
            self.assertEqual(rows[1]["original"], "café")
            self.assertEqual(rows[1]["encoded"], "caf%C3%A9")
            self.assertEqual(rows[3]["state"], "duplicate")
            bad = ["", "white space", "a\tb", "../x", ".", "..", "a..b", "a/b", "a\\b",
                   "https:x", "%2F", "a?b", "a#b", "a&b", "a=b", "a\x00b"]
            (root / "top.txt").write_text("\n".join(bad) + "\nok\n", encoding="utf-8")
            rejected = root / "rejected"
            result = invoke(PREPARE, *args, "--out", rejected)
            self.assertEqual(result.returncode, 2)
            manifest = json.loads((rejected / "manifest.json").read_text())
            self.assertEqual(manifest["rejected_rows"], len(bad))
            self.assertEqual(manifest["inputs"]["top"]["rows"], len(bad) + 1)
            self.assertEqual(len(json.loads((rejected / "rejects.json").read_text())), len(bad))
            (root / "top.txt").write_bytes(b"bad\xff\n")
            invalid_utf8 = root / "invalid-utf8"
            self.assertEqual(invoke(PREPARE, *args, "--out", invalid_utf8).returncode, 2)
            self.assertFalse(invalid_utf8.exists())


class GuardTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.base = "https://scope.invalid/blog/assets/plugins/"
        self.words = self.root / "words.txt"
        self.words.write_text("first\nde_de\ncaf%C3%A9\nfourth\nfifth\n")
        self.auth_path, self.cal_path = self.root / "auth.json", self.root / "cal.json"
        self.profile = {"proxy": None, "user_agent": "WordPress-Pentest/1.0"}
        self.auth = {"base_url": self.base, "automated_discovery": True,
                     "request_profile": dict(self.profile),
                     "scope_ref": "client-authorization", "actor_session_ref": "anonymous",
                     "valid_until": (datetime.now(timezone.utc) + timedelta(hours=1)).isoformat(),
                     "max_requests": 20, "max_rate": 50, "max_seconds": 20}
        def control(slug):
            return {"slug": slug, "triad_refs": {p: "private/raw-" + slug + "-" + p
                                                  for p in ("slashless", "folder", "readme")}}
        self.cal = {"schema_version": 1, "base_url": self.base, "method": "GET",
                    "request_profile": dict(self.profile),
                    "probe": "slashless", "reliable": True,
                    "timestamp": datetime.now(timezone.utc).isoformat(),
                    "canonical_ref": "private/canonical", "encoded_control_ref": "private/encoded",
                    "known_positive": dict(control("known"), asset_ref="private/frontpage-asset"),
                    "absence_controls": [control("random-" + str(n)) for n in range(3)]}
        self.fake = self.root / "fake-ffuf"
        self.fake.write_text("#!" + sys.executable + "\n" + '''
import base64, json, os, pathlib, sys, time
args = sys.argv[1:]
pathlib.Path(__file__).with_suffix('.invocation').write_text(json.dumps({
    'args': args, 'home': os.environ.get('HOME'), 'xdg': os.environ.get('XDG_CONFIG_HOME'),
    'proxy_env': {k: v for k, v in os.environ.items()
                  if k.lower() in {'http_proxy', 'https_proxy', 'all_proxy', 'no_proxy'}}}))
words = pathlib.Path(args[args.index('-w') + 1]).read_text().splitlines()
base = args[args.index('-u') + 1]
statuses = json.loads(pathlib.Path(__file__).with_suffix('.statuses').read_text())
for n, status in enumerate(statuses, 1):
    if status == 'hang':
        time.sleep(60)
    elif status == 'invalid':
        print('{broken json', flush=True)
    elif status == 'error':
        print(json.dumps({'error': 'transport failed'}), flush=True)
    else:
        print(json.dumps({'status': status, 'position': n,
                          'input': {'FUZZ': base64.b64encode(words[n-1].encode()).decode()},
                          'url': base.replace('FUZZ', words[n-1])}), flush=True)
    time.sleep(0.05)
time.sleep(60)
''')
        self.fake.chmod(0o700)
        self.set_statuses([200, 301, 404, 200, 200])

    def set_statuses(self, statuses):
        self.fake.with_suffix(".statuses").write_text(json.dumps(statuses))

    def command(self, out="run", **options):
        self.auth_path.write_text(json.dumps(self.auth))
        self.cal_path.write_text(json.dumps(self.cal))
        args = {"base-url": self.base, "authorization": self.auth_path, "calibration": self.cal_path,
                "wordlist": self.words, "out": self.root / out, "max-requests": 5,
                "max-seconds": 5, "ffuf-path": self.fake}
        args.update(options)
        return [sys.executable, str(GUARD)] + [str(v) for k, val in args.items() for v in ("--" + k, val)]

    def run_guard(self, out="run", **options):
        result = subprocess.run(self.command(out, **options), capture_output=True, text=True, timeout=10)
        path = self.root / out / "summary.json"
        return result, json.loads(path.read_text()) if path.exists() else None

    def test_completed_batch_and_existing_output_refusal(self):
        result, summary = self.run_guard()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(summary["state"], "COMPLETED_BATCH_UNVERIFIED")
        self.assertEqual(summary["attempted_count"], 5)
        self.assertEqual(summary["statuses"]["404"], 1)
        self.assertEqual(summary["rate"], 1)
        self.assertFalse(summary["attempted_count_is_lower_bound"])
        invocation = json.loads(self.fake.with_suffix(".invocation").read_text())
        args = invocation["args"]
        self.assertEqual(args[args.index("-mc") + 1], "all")
        self.assertEqual(args[args.index("-t") + 1], "1")
        self.assertEqual(args[args.index("-rate") + 1], "1")
        self.assertEqual(args[args.index("-H") + 1], "User-Agent: WordPress-Pentest/1.0")
        self.assertEqual(args.count("-H"), 1)
        self.assertNotIn("-x", args)
        self.assertEqual(summary["request_profile"], self.profile)
        self.assertEqual(invocation["proxy_env"], {})
        for flag in ("-json", "-noninteractive", "-se", "-sf", "-raw",
                     "-r=false", "-ac=false", "-recursion=false"):
            self.assertIn(flag, args)
        for forbidden in ("-r", "-ac", "-fc", "-fs", "-fw", "-fl", "-fr", "-recursion"):
            self.assertNotIn(forbidden, args)
        self.assertEqual(invocation["home"], str((self.root / "run/ffuf-home").resolve()))
        self.assertEqual(invocation["xdg"], invocation["home"])
        before = (self.root / "run/ffuf.jsonl").read_bytes()
        again, _ = self.run_guard()
        self.assertEqual(again.returncode, 2)
        self.assertEqual((self.root / "run/ffuf.jsonl").read_bytes(), before)
        self.assertEqual((self.root / "run").stat().st_mode & 0o777, 0o700)

    def test_non_posix_refused_before_artifacts_outputs_or_subprocess(self):
        spec = importlib.util.spec_from_file_location("wordpress_ffuf_guard", GUARD)
        guard = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(guard)
        command = self.command()
        error = io.StringIO()
        with (mock.patch.object(guard, "is_posix", return_value=False),
              mock.patch.object(guard, "load_object") as load,
              mock.patch.object(guard.Path, "mkdir") as mkdir,
              mock.patch.object(guard.subprocess, "Popen") as popen,
              mock.patch.object(sys, "argv", command[1:]), redirect_stderr(error)):
            self.assertEqual(guard.main(), 2)
            load.assert_not_called()
            mkdir.assert_not_called()
            popen.assert_not_called()
        self.assertIn("requires POSIX (Linux/macOS)", error.getvalue())
        self.assertFalse((self.root / "run").exists())
        self.assertFalse(self.fake.with_suffix(".invocation").exists())

    def test_direct_mode_user_agent_and_proxy_environment_isolation(self):
        profile = dict(self.profile, user_agent="Authorized-WordPress-Review/2.0 test")
        self.auth["request_profile"] = dict(profile)
        self.cal["request_profile"] = dict(profile)
        # Fake ffuf only: inherited proxy endpoints are never contacted.
        with mock.patch.dict("os.environ", {"HTTP_PROXY": "http://unwanted.invalid:8080",
                                            "https_proxy": "http://unwanted.invalid:8081",
                                            "ALL_PROXY": "socks5://unwanted.invalid:1080",
                                            "no_proxy": "*"}):
            result, summary = self.run_guard(**{"user-agent": profile["user_agent"]})
        self.assertEqual(result.returncode, 0, result.stderr)
        invocation = json.loads(self.fake.with_suffix(".invocation").read_text())
        args = invocation["args"]
        self.assertNotIn("-x", args)
        self.assertNotIn("-replay-proxy", args)
        self.assertEqual(args[args.index("-H") + 1], "User-Agent: " + profile["user_agent"])
        self.assertEqual(args.count("-H"), 1)
        self.assertEqual(summary["request_profile"], profile)
        self.assertEqual(invocation["proxy_env"], {})

    def test_proxy_option_refuses_before_spawn(self):
        result, summary = self.run_guard(**{"proxy": "http://127.0.0.1:8080"})
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("unrecognized arguments: --proxy", result.stderr)
        self.assertIsNone(summary)
        self.assertFalse((self.root / "run").exists())
        self.assertFalse(self.fake.with_suffix(".invocation").exists())

    def test_request_profile_missing_or_mismatched_refuses_before_spawn(self):
        cases = [({}, self.profile, {}),
                 (self.profile, {}, {}),
                 ({"proxy": None}, self.profile, {}),
                 (dict(self.profile, user_agent="different"), self.profile, {}),
                 (self.profile, dict(self.profile, user_agent="different"), {}),
                 (self.profile, self.profile, {"user-agent": "different"}),
                  (dict(self.profile, proxy="http://127.0.0.1:8080"), self.profile, {}),
                  (self.profile, dict(self.profile, proxy="http://127.0.0.1:8080"), {}),
                 (dict(self.profile, proxy="http://127.0.0.1:8080"),
                  dict(self.profile, proxy="http://127.0.0.1:8080"), {}),
                 (dict(self.profile, extra="not-supported"), self.profile, {})]
        for number, (auth_profile, cal_profile, options) in enumerate(cases):
            self.auth["request_profile"] = dict(auth_profile)
            self.cal["request_profile"] = dict(cal_profile)
            out = "mismatch" + str(number)
            result, summary = self.run_guard(out, **options)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn("request_profile must exactly match", result.stderr)
            self.assertIsNone(summary)
            self.assertFalse((self.root / out).exists())
        self.assertFalse(self.fake.with_suffix(".invocation").exists())

    def test_invalid_user_agents_refuse_before_spawn(self):
        for number, ua in enumerate(["", "   ", "agent\r\nX-Injected: yes", "agent\tvalue", "agent\x7f"]):
            out = "bad-agent" + str(number)
            result, summary = self.run_guard(out, **{"user-agent": ua})
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIsNone(summary)
            self.assertFalse((self.root / out).exists())
        self.assertFalse(self.fake.with_suffix(".invocation").exists())

    def test_stop_conditions_are_streamed_not_filtered(self):
        cases = [([404, 429, 200], "first_429", 2),
                 ([500, 200, 502, 200], "second_unexpected_5xx", 3),
                 ([403, 200, 403, 200], "repeated_challenge_like_status", 3),
                 ([406, 406, 200], "repeated_challenge_like_status", 2),
                 ([200, "invalid", 200], "json_error", 1),
                 (["error"], "json_error", 0),
                 ([0], "json_error", 0)]
        for n, (statuses, reason, records) in enumerate(cases):
            with self.subTest(reason=reason):
                self.set_statuses(statuses)
                result, summary = self.run_guard("case" + str(n))
                self.assertEqual(result.returncode, 3, result.stderr)
                self.assertEqual(summary["state"], "BLOCKED")
                self.assertEqual(summary["stop_reason"], reason)
                self.assertEqual(summary["result_count"], records)
                self.assertTrue(summary["attempted_count_is_lower_bound"])
        self.assertIn(b'429', (self.root / "case0/ffuf.jsonl").read_bytes())

    def test_request_time_and_calibration_budgets(self):
        result, summary = self.run_guard("budget", **{"max-requests": 2})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(summary["scheduled_count"], 2)
        self.assertEqual((self.root / "budget/batch.encoded.txt").read_text(), "first\nde_de\n")
        self.set_statuses(["hang"])
        result, summary = self.run_guard("timeout", **{"max-seconds": 1})
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertEqual(summary["stop_reason"], "time_budget")
        self.cal["timestamp"] = (datetime.now(timezone.utc) - timedelta(seconds=1)).isoformat()
        result, summary = self.run_guard("stale-during-run", **{"calibration-max-age": 2})
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertEqual(summary["stop_reason"], "calibration_expired")

    def test_refuses_missing_authorization_bad_paths_and_unsafe_limits_before_spawn(self):
        for n, base in enumerate(["https://u:p@scope.invalid/x/", "https://scope.invalid/x/?q=x",
                                  "https://scope.invalid/x/#", "file:///tmp/", self.base + "FUZZ/",
                                  "https://scope.invalid/x/%2e%2e/", "https://scope.invalid/x/%2F/",
                                  "https://scope.invalid/x", "https://scope.invalid:bad/x/"]):
            with self.subTest(base=base):
                result, summary = self.run_guard("url" + str(n), **{"base-url": base})
                self.assertEqual(result.returncode, 2)
                self.assertIsNone(summary)
                self.assertFalse((self.root / ("url" + str(n))).exists())
        for n, opts in enumerate([{"rate": 51}, {"rate": 0}, {"rate": "nan"},
                                  {"max-requests": -1}, {"max-seconds": 0},
                                  {"max-requests": 21}, {"calibration-max-age": 901},
                                  {"ffuf-path": str(self.root / "missing")},
                                  {"authorization": self.root / "missing-auth"}]):
            result, summary = self.run_guard("limit" + str(n), **opts)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIsNone(summary)
        self.cal["reliable"] = False
        self.assertEqual(self.run_guard("unreliable")[0].returncode, 2)
        self.cal["reliable"] = True
        self.cal["timestamp"] = (datetime.now(timezone.utc) - timedelta(hours=1)).isoformat()
        self.assertEqual(self.run_guard("stale")[0].returncode, 2)
        self.cal["timestamp"] = datetime.now(timezone.utc).isoformat()
        self.auth["actor_session_ref"] = "admin-session"
        self.assertEqual(self.run_guard("wrong-actor")[0].returncode, 2)
        self.auth["actor_session_ref"] = "anonymous"
        self.cal["absence_controls"][1] = self.cal["absence_controls"][0]
        self.assertEqual(self.run_guard("duplicate-controls")[0].returncode, 2)
        self.cal["absence_controls"][1] = {
            "slug": "random-1", "triad_refs": {p: "private/raw" for p in ("slashless", "folder", "readme")}}
        self.cal.pop("encoded_control_ref")
        self.assertEqual(self.run_guard("encoded-without-control")[0].returncode, 2)

    def test_encoded_input_boundaries_and_plaintext_stream_contract(self):
        for n, word in enumerate(["../x", "a/b", "%252F", "%2F", "a%3Fb", "%ff", "a b",
                                  "caf%c3%a9", "café", "a\na", "%00", ""]):
            self.words.write_text(word + "\n")
            result, summary = self.run_guard("word" + str(n))
            self.assertEqual(result.returncode, 2, (word, result.stderr))
            self.assertIsNone(summary)
        self.words.write_text("Zmlyc3Q=\n")  # do not guess EJSON/base64 interpretation
        self.assertEqual(self.run_guard("base64-not-slug")[0].returncode, 2)
        self.words.write_text("%E2%80%8Bsearch-excel-csv\nZm9v\n")
        self.assertEqual(self.run_guard("format-preserved", **{"max-requests": 2})[0].returncode, 0)
        self.fake.write_text(self.fake.read_text().replace(
            "base64.b64encode(words[n-1].encode()).decode()", "words[n-1]"))
        self.assertEqual(self.run_guard("plaintext-explicit", **{
            "max-requests": 2, "json-input-encoding": "plaintext"})[0].returncode, 0)
        result, summary = self.run_guard("wrong-stream-schema", **{"max-requests": 2})
        self.assertEqual(result.returncode, 3)
        self.assertEqual(summary["stop_reason"], "json_error")

    def test_cancellation_terminates_the_actual_process_group(self):
        # A child ignores TERM and retains no pipes; the supervisor must KILL
        # the process group even if ffuf's parent exits immediately on TERM.
        self.fake.write_text("#!" + sys.executable + "\n" + '''
import os, pathlib, signal, subprocess, sys, time
child = subprocess.Popen([sys.executable, '-c',
    'import signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(60)'],
    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
pathlib.Path(__file__).with_suffix('.pids').write_text(str(os.getpid()) + ' ' + str(child.pid))
time.sleep(60)
''')
        cancel = self.root / "cancel"
        command = self.command(**{"cancel-file": cancel})
        process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.addCleanup(lambda: process.poll() is None and process.kill())
        pids_path = self.fake.with_suffix(".pids")
        deadline = time.monotonic() + 3
        while not pids_path.exists() and time.monotonic() < deadline:
            time.sleep(0.02)
        self.assertTrue(pids_path.exists())
        parent, child = map(int, pids_path.read_text().split())
        cancel.touch()
        _, stderr = process.communicate(timeout=5)
        self.assertEqual(process.returncode, 3, stderr)
        summary = json.loads((self.root / "run/summary.json").read_text())
        self.assertEqual(summary["stop_reason"], "cancelled")
        for pid in (parent, child):
            # An orphan zombie may await the container's init; it cannot run.
            status = Path(f"/proc/{pid}/stat")
            if status.exists():
                self.assertEqual(status.read_text().split()[2], "Z", f"process {pid} still running")
        result, summary = self.run_guard("pre-cancel", **{"cancel-file": cancel})
        self.assertEqual(result.returncode, 2)
        self.assertIsNone(summary)

    @unittest.skipUnless(shutil.which("ffuf"), "installed ffuf unavailable")
    def test_installed_ffuf_base64_jsonl_encoding_and_no_redirects(self):
        requests = []
        user_agents = []
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                requests.append(self.path)
                user_agents.append(self.headers.get("User-Agent"))
                self.send_response(301 if len(requests) == 1 else 429)
                self.send_header("Location", "/must-not-follow")
                self.end_headers()
                self.wfile.write(b"local fixture only")

            def log_message(self, *_args):
                pass

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.addCleanup(server.server_close)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.shutdown)
        self.base = f"http://127.0.0.1:{server.server_port}/blog/content/plugins/"
        self.auth["base_url"] = self.cal["base_url"] = self.base
        self.words.write_text("caf%C3%A9\nde_de\nthird\n")
        result, summary = self.run_guard("installed", **{"ffuf-path": shutil.which("ffuf"),
                                                        "max-requests": 3, "max-seconds": 10})
        self.assertEqual(result.returncode, 3, result.stderr)
        self.assertEqual(summary["stop_reason"], "first_429",
                         (self.root / "installed/ffuf.jsonl").read_text())
        wire_paths = [urlsplit(path).path for path in requests]
        self.assertEqual(wire_paths[:2], ["/blog/content/plugins/caf%C3%A9", "/blog/content/plugins/de_de"])
        self.assertLessEqual(len(requests), 3)
        self.assertNotIn("/must-not-follow", wire_paths)
        self.assertTrue(user_agents)
        self.assertEqual(set(user_agents), {"WordPress-Pentest/1.0"})
        raw = [json.loads(line) for line in (self.root / "installed/ffuf.jsonl").read_text().splitlines()]
        self.assertEqual(base64.b64decode(raw[0]["input"]["FUZZ"], validate=True).decode(), "caf%C3%A9")
        self.assertIn(429, [row["status"] for row in raw])


if __name__ == "__main__":
    unittest.main()
