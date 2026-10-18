"""Failure redaction plus opt-in lifecycle checks against actual owned containers."""
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import time
import unittest

RUNNER = Path(__file__).with_name("run.py")


def unused_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


class DisposableRunnerTest(unittest.TestCase):
    def test_pytest_failure_never_discloses_private_bootstrap_details(self):
        secret = "PRIVATE-BOOTSTRAP-SENTINEL-DO-NOT-DISCLOSE"
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)
            (source / "src").mkdir()
            fixture_dir = source / "tests/api"
            fixture_dir.mkdir(parents=True)
            (fixture_dir / "conftest.py").write_text(
                "import pytest\n"
                "def startup():\n"
                f"    print({secret!r})\n"
                f"    pytest.fail({secret!r})\n"
                "    yield 'unused'\n"
                "class Fixture:\n"
                "    __wrapped__ = staticmethod(startup)\n"
                "docker_container = Fixture()\n"
            )
            result = subprocess.run(
                [sys.executable, str(RUNNER), "--source", str(source), "--image", "unused", "--", sys.executable, "-c", "pass"],
                capture_output=True, text=True, timeout=30,
            )
            self.assertEqual(result.returncode, 1)
            self.assertNotIn(secret, result.stdout + result.stderr)
            self.assertNotIn("Traceback", result.stdout + result.stderr)
            self.assertIn("Disposable acceptance setup failed", result.stderr)

    @unittest.skipUnless(os.environ.get("DISPOSABLE_CORE_SOURCE") and os.environ.get("DISPOSABLE_CORE_IMAGE"), "set source/image to run the actual disposable Core SIGTERM test")
    def test_sigterm_cleans_actual_backend_vaults_ssh_and_test_process(self):
        process = None
        owned = []
        child_pid = None
        with tempfile.TemporaryDirectory() as directory:
            ready = Path(directory) / "ready.json"
            code = (
                "import json,os,pathlib,time,urllib.parse;"
                "vaults=json.loads(pathlib.Path(os.environ['MISTERSHELL_ACC_VAULT_FIXTURES']).read_text());"
                "names=[os.environ['MISTERSHELL_TEST_CONTAINER_NAME'],os.environ['MISTERSHELL_ACC_SSH_UPDATE_HOST']]+"
                "[urllib.parse.urlparse(v['backend_url']).hostname for v in vaults.values()];"
                f"pathlib.Path({str(ready)!r}).write_text(json.dumps({{'names':names,'pid':os.getpid()}}));"
                "time.sleep(600)"
            )
            with (Path(directory) / "runner.log").open("w+") as log:
                process = subprocess.Popen([
                    sys.executable, str(RUNNER), "--source", os.environ["DISPOSABLE_CORE_SOURCE"],
                    "--image", os.environ["DISPOSABLE_CORE_IMAGE"], "--api-port", str(unused_port()),
                    "--ssh-port", str(unused_port()), "--", sys.executable, "-c", code,
                ], stdout=log, stderr=log, start_new_session=True)
                try:
                    deadline = time.monotonic() + 240
                    while not ready.exists() and process.poll() is None and time.monotonic() < deadline:
                        time.sleep(0.2)
                    self.assertTrue(ready.exists(), "disposable runner did not reach the test command")
                    details = json.loads(ready.read_text())
                    owned, child_pid = details["names"], details["pid"]
                    self.assertEqual(len(owned), 4)
                    process.send_signal(signal.SIGTERM)
                    process.wait(timeout=60)
                    self.assertEqual(process.returncode, 128 + signal.SIGTERM)
                    for name in owned:
                        result = subprocess.run(["docker", "inspect", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                        self.assertNotEqual(result.returncode, 0, f"owned container survived SIGTERM: {name}")
                    with self.assertRaises(ProcessLookupError):
                        os.kill(child_pid, 0)
                finally:
                    if process is not None:
                        try:
                            os.killpg(process.pid, signal.SIGTERM)
                        except ProcessLookupError:
                            pass
                        if process.poll() is None:
                            process.wait(timeout=60)
                    if child_pid:
                        try:
                            os.kill(child_pid, signal.SIGTERM)
                        except ProcessLookupError:
                            pass
                    if owned:
                        subprocess.run(["docker", "rm", "-f", *owned], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
