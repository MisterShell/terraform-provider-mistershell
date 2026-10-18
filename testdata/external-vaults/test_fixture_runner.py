"""Integration checks using real isolated Docker containers (no production backend)."""

import json
from pathlib import Path
import secrets
import signal
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from urllib.error import HTTPError
from urllib.request import Request, urlopen


RUNNER = Path(__file__).with_name("run_fixtures.py")


def request(url, path, token=None, body=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["X-Vault-Token"] = token
    req = Request(url + "/v1/" + path, headers=headers,
                  data=None if body is None else json.dumps(body).encode())
    with urlopen(req, timeout=5) as response:
        raw = response.read()
        return json.loads(raw) if raw else {}


class FixtureRunnerTest(unittest.TestCase):
    def test_host_urls_sigint_and_existing_file_protection(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "fixtures.json"
            process = subprocess.Popen(
                [sys.executable, str(RUNNER), "--output", str(output)],
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
            )
            try:
                deadline = time.monotonic() + 60
                while not output.exists() and process.poll() is None and time.monotonic() < deadline:
                    time.sleep(0.1)
                self.assertTrue(output.exists(), "fixture runner did not publish metadata")
                original = output.read_bytes()
                fixtures = json.loads(original)
                for fixture in fixtures.values():
                    self.assertEqual(fixture["backend_url"], fixture["url"])
                duplicate = subprocess.run(
                    [sys.executable, str(RUNNER), "--output", str(output)],
                    capture_output=True, text=True, timeout=10,
                )
                self.assertNotEqual(duplicate.returncode, 0)
                self.assertEqual(output.read_bytes(), original)
                process.send_signal(signal.SIGINT)
                log, _ = process.communicate(timeout=30)
                self.assertEqual(process.returncode, 0, log)
                self.assertFalse(output.exists())
                for fixture in fixtures.values():
                    with self.assertRaises(Exception):
                        request(fixture["url"], "sys/health")
            finally:
                if process.poll() is None:
                    process.send_signal(signal.SIGTERM)
                    process.communicate(timeout=30)

    def test_real_vaults_readonly_approle_and_signal_cleanup(self):
        network = "tf-vault-fixture-test-" + secrets.token_hex(6)
        subprocess.run(["docker", "network", "create", network], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        process = None
        container_ids = []
        try:
            with tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / "fixtures.json"
                process = subprocess.Popen(
                    [sys.executable, str(RUNNER), "--network", network, "--output", str(output)],
                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
                )
                deadline = time.monotonic() + 60
                while not output.exists() and process.poll() is None and time.monotonic() < deadline:
                    time.sleep(0.1)
                self.assertTrue(output.exists(), "fixture runner did not publish metadata")
                # Publication is atomic: existence means complete JSON is available.
                fixtures = json.loads(output.read_text())
                self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
                self.assertEqual(set(fixtures), {"hashicorp", "openbao"})
                for kind, fixture in fixtures.items():
                    self.assertEqual(set(fixture), {"url", "backend_url", "token", "role_id", "secret_id"})
                    self.assertTrue(fixture["url"].startswith("http://127.0.0.1:"))
                    name = fixture["backend_url"].removeprefix("http://").removesuffix(":8200")
                    container_ids.append(subprocess.check_output(
                        ["docker", "inspect", "--format", "{{.Id}}", name], text=True).strip())
                    self.assertEqual(request(fixture["url"], "sys/health")["sealed"], False)
                    request(fixture["url"], "secret/data/fixture-check", fixture["token"],
                            {"data": {"password": "fixture-only-value"}})
                    if kind == "openbao":
                        auth = request(fixture["url"], "auth/approle/login", body={
                            "role_id": fixture["role_id"], "secret_id": fixture["secret_id"],
                        })
                        reader = auth["auth"]["client_token"]
                        data = request(fixture["url"], "secret/data/fixture-check", reader)
                        self.assertEqual(data["data"]["data"]["password"], "fixture-only-value")
                        with self.assertRaises(HTTPError) as forbidden:
                            request(fixture["url"], "secret/data/fixture-check", reader,
                                    {"data": {"password": "should-not-write"}})
                        self.assertEqual(forbidden.exception.code, 403)
                process.send_signal(signal.SIGTERM)
                log, _ = process.communicate(timeout=30)
                self.assertEqual(process.returncode, 0, log)
                self.assertFalse(output.exists(), "secret metadata survived cleanup")
                for fixture in fixtures.values():
                    for key in ("token", "secret_id"):
                        if fixture[key]:
                            self.assertNotIn(fixture[key], log)
                for container in container_ids:
                    inspected = subprocess.run(["docker", "inspect", container], capture_output=True)
                    self.assertNotEqual(inspected.returncode, 0, "owned container survived cleanup")
        finally:
            if process is not None and process.poll() is None:
                process.send_signal(signal.SIGTERM)
                process.communicate(timeout=30)
            subprocess.run(["docker", "network", "rm", network],
                           stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, check=False)


if __name__ == "__main__":
    unittest.main()
