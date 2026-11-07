"""The collector must stay stopped outside an authorized running scope."""
import datetime
import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch


class StackScopeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location("docker_stack", Path(__file__).resolve().parents[1] / "docker/stack.py")
        cls.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.module)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.session = Path(self.temp.name)
        self.stack = self.module.Stack(self.session)

    def test_native_host_follows_the_actual_chromium_profile(self):
        source = self.session / "manifest-source.json"
        origins = ["chrome-extension://trusted-extension/"]
        source.write_text(json.dumps({"name": "com.interceptor.host", "path": "__DAEMON_PATH__", "type": "stdio", "allowed_origins": origins}))
        profile = self.stack.root / "chromium"
        destination = self.module.configure_native_host(profile, source)
        self.assertEqual(destination, profile / "NativeMessagingHosts/com.interceptor.host.json")
        manifest = json.loads(destination.read_text())
        self.assertEqual(manifest["allowed_origins"], origins)
        self.assertEqual(manifest["path"], "/opt/interceptor/daemon/interceptor-daemon")
        self.assertEqual(destination.stat().st_mode & 0o777, 0o600)
        self.assertFalse((self.stack.root / "browser-home/.config/chromium/NativeMessagingHosts").exists())

    def test_daemon_cleanup_timeout_does_not_mask_the_job_result(self):
        self.stack.owns_lock = True
        handle = open(self.session / "test.log", "w")
        self.stack.handles.append(handle)
        with patch.object(self.module.subprocess, "run", side_effect=subprocess.TimeoutExpired("interceptor", 15)):
            self.stack.close()
        self.assertTrue(handle.closed)

    def state(self, mode="running", **scope):
        data = {"mode": mode, "scope": {"origins": ["https://example.test:8443"], "methods": ["GET"], **scope}}
        (self.session / "state.json").write_text(json.dumps(data))

    def test_no_scope_idle_paused_or_expired_cannot_collect(self):
        self.assertIsNone(self.stack.scope())
        for mode in ["idle", "paused", "complete"]:
            self.state(mode=mode)
            self.assertIsNone(self.stack.scope())
        self.state(expiresAt="2000-01-01T00:00:00Z")
        self.assertIsNone(self.stack.scope())
        self.state(methods=["POST"])
        self.assertIsNone(self.stack.scope())

    def test_exact_scoped_origins_keep_explicit_ports(self):
        self.state()
        self.assertEqual(self.stack.scope(), ("https://example.test:8443",))

    def test_collection_does_not_start_without_scope(self):
        with patch.object(self.stack, "start") as start:
            self.stack.update_collection()
            start.assert_not_called()

    def test_pause_stops_both_collection_processes(self):
        self.stack.scope_key = ("https://example.test:8443",)
        self.state(mode="paused")
        with patch.object(self.stack, "stop") as stop, patch.object(self.stack, "start") as start:
            self.stack.update_collection()
            self.assertEqual([c.args[0] for c in stop.call_args_list], ["bridge", "jxscout"])
            start.assert_not_called()


if __name__ == "__main__":
    unittest.main()
