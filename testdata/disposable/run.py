#!/usr/bin/env python3
"""Run provider acceptance tests against an owned, disposable MisterShell Core.

Requires the MisterShell source checkout's Python test environment and a current
Core test image built with its test license key. Never uses a development API.
"""
import argparse
import contextlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import signal
import subprocess
import sys
import tempfile
import time
import tomllib
from urllib.parse import urlparse

import httpx

OWNER_LABEL = "mistershell.terraform.acceptance.owner"


class RunInterrupted(BaseException):
    def __init__(self, signum):
        self.signum = signum


def interrupt(signum, _frame):
    raise RunInterrupted(signum)


def docker(*args):
    result = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError("Docker fixture operation failed; check the required local images")
    return result.stdout.strip()


def stop_process(process, *, group=False):
    if process is None:
        return
    try:
        if group:
            os.killpg(process.pid, signal.SIGTERM)
        elif process.poll() is None:
            process.send_signal(signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=30)
    except subprocess.TimeoutExpired:
        if group:
            os.killpg(process.pid, signal.SIGKILL)
        else:
            process.kill()
        process.wait(timeout=10)


def cleanup(owner, core, ssh, backend, vault_process, test_process, vault_file, log):
    # A second Ctrl-C/SIGTERM must not interrupt removal of already owned trust,
    # secrets, containers or child processes.
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, signal.SIG_IGN)
    failures = []

    def attempt(action):
        try:
            action()
        except BaseException:
            # Even fixture teardown can use pytest.fail (a BaseException).
            # Never expose its message or traceback, and continue other cleanup.
            failures.append(True)

    vault_names = []
    if vault_file is not None and vault_file.exists():
        try:
            for item in json.loads(vault_file.read_text()).values():
                name = urlparse(item["backend_url"]).hostname
                if name and name.startswith("tf-mistershell-vault-"):
                    vault_names.append(name)
        except (OSError, ValueError, KeyError, TypeError):
            failures.append(True)
    attempt(lambda: stop_process(test_process, group=True))
    attempt(lambda: stop_process(vault_process))
    if backend is not None:
        with contextlib.redirect_stdout(log), contextlib.redirect_stderr(log):
            attempt(backend.close)

    # Core startup can fail before the fixture yields. Remove by our generated
    # identity regardless of whether its generator reached its teardown code.
    names = [ssh, core, *vault_names]
    try:
        names.extend(docker("ps", "-aq", "--filter", f"label={OWNER_LABEL}={owner}").split())
    except Exception:
        failures.append(True)
    attempt(lambda: subprocess.run(["docker", "rm", "-f", *names], stdout=subprocess.DEVNULL,
                                   stderr=subprocess.DEVNULL, timeout=60))
    if failures:
        print("Some owned fixture cleanup operations failed; check Docker and child-process state.", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--image", required=True)
    parser.add_argument("--api-port", default=18889, type=int)
    parser.add_argument("--ssh-port", default=18823, type=int)
    parser.add_argument("--ssh-image", default="terraform-provider-mistershell-ssh-linux:latest")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        parser.error("provide the test command after --")
    if args.api_port in {3000, 5432, 6379, 8000, 8001} or args.ssh_port in {3000, 5432, 6379, 8000, 8001}:
        parser.error("reserved development ports are forbidden")
    previous_handlers = {sig: signal.signal(sig, interrupt) for sig in (signal.SIGINT, signal.SIGTERM)}
    source = args.source.resolve()
    owner = "tfacc-" + secrets.token_hex(6)
    core, ssh = owner + "-core", owner + "-ssh"
    os.environ.update(MISTERSHELL_TEST_CONTAINER_NAME=core,
                      MISTERSHELL_TEST_CORE_IMAGE=args.image,
                      MISTERSHELL_TEST_API_PORT=str(args.api_port),
                      MISTERSHELL_TEST_SSH_PORT=str(args.ssh_port))
    backend = vault_process = test_process = vault_file = None
    try:
        with tempfile.TemporaryDirectory(prefix=owner + "-") as directory:
            directory = Path(directory)
            log_path = directory / "bootstrap.log"
            with log_path.open("w") as log:
                os.chmod(log_path, 0o600)
                try:
                    print("Starting isolated MisterShell acceptance fixtures", flush=True)
                    # Imports, fixture exceptions and setup SQL can include
                    # credentials in diagnostics. Keep all bootstrap output private.
                    with contextlib.redirect_stdout(log), contextlib.redirect_stderr(log):
                        sys.path.insert(0, str(source / "src"))
                        spec = importlib.util.spec_from_file_location("disposable_fixture", source / "tests/api/conftest.py")
                        fixture = importlib.util.module_from_spec(spec)
                        spec.loader.exec_module(fixture)
                        backend = fixture.docker_container.__wrapped__()
                        base = next(backend)
                        with httpx.Client(base_url=base, verify=False, timeout=30) as api:
                            response = api.post("/api/v1/auth/login", data={"username": "admin@mistershell.local", "password": fixture._ADMIN_PASSWORD})
                            response.raise_for_status()
                            api.headers["Authorization"] = "Bearer " + response.json()["data"]["access_token"]
                            response = api.post("/api/v1/auth/api-key", json={})
                            response.raise_for_status()
                            key = response.json()["data"]["key"]
                            response = api.get("/openapi.json")
                            response.raise_for_status()
                            expected = tomllib.loads((source / "pyproject.toml").read_text())["project"]["version"]
                            if response.json()["info"]["version"] != expected:
                                raise RuntimeError("Test image version does not match the source checkout")
                            for feature in ("ids", "proxy", "collector"):
                                sql, _ = fixture._license_insert(feature, 100)
                                fixture._run_psql(sql)
                            fixture._run_psql("INSERT INTO sensor_ruleset_sources(name, source_type, enabled, url) VALUES ('tfacc-index-fixture','index',false,'https://example.invalid/never-fetch.rules')")
                            fixture._run_psql("INSERT INTO fact_policy_checks(name, fact_type, is_builtin, enabled) VALUES ('tfacc-builtin-fact-fixture','system_ntp',true,false)")
                            response = api.put("/api/v1/settings/licensing_email", json={"value": fixture.FIXTURE_LICENSING_EMAIL})
                            response.raise_for_status()
                        docker("run", "-d", "--name", ssh, "--label", f"{OWNER_LABEL}={owner}", "--network", "mistershell_test_network", args.ssh_image)
                        address = docker("inspect", "--format", '{{(index .NetworkSettings.Networks "mistershell_test_network").IPAddress}}', ssh)
                        vault_file = directory / "vaults.json"
                        vault_script = Path(__file__).resolve().parents[1] / "external-vaults/run_fixtures.py"
                        vault_process = subprocess.Popen([sys.executable, str(vault_script), "--network", "mistershell_test_network", "--output", str(vault_file)], stdout=log, stderr=log, start_new_session=True)
                        deadline = time.monotonic() + 60
                        while not vault_file.exists():
                            if vault_process.poll() is not None or time.monotonic() > deadline:
                                raise RuntimeError("Vault fixtures did not become ready")
                            time.sleep(0.1)
                        env = os.environ.copy()
                        env.update(TF_ACC="1", MISTERSHELL_URL=base, MISTERSHELL_API_KEY=key,
                                   MISTERSHELL_INSECURE="true", MISTERSHELL_ACC_SSH_HOST=address,
                                   MISTERSHELL_ACC_SSH_PORT="22", MISTERSHELL_ACC_SSH_USERNAME="tfacc",
                                   MISTERSHELL_ACC_SSH_PASSWORD="tfacc-password", MISTERSHELL_ACC_SSH_UPDATE_HOST=ssh,
                                   MISTERSHELL_ACC_GENERIC_SSH_BIND_ADDRESS="0.0.0.0:0",
                                   MISTERSHELL_ACC_GENERIC_SSH_HOST="host.docker.internal",
                                   MISTERSHELL_ACC_VAULT_FIXTURES=str(vault_file),
                                   MISTERSHELL_ACC_RUN_OWNER=owner,
                                   MISTERSHELL_ACC_SENSOR_RULESET_UPLOAD="1")
                    print("Running tests against disposable Core " + expected, flush=True)
                    test_process = subprocess.Popen(command, env=env, start_new_session=True)
                    return test_process.wait()
                finally:
                    cleanup(owner, core, ssh, backend, vault_process, test_process, vault_file, log)
    finally:
        for sig, handler in previous_handlers.items():
            signal.signal(sig, handler)


def cli():
    try:
        return main()
    except RunInterrupted as interrupted:
        print("Disposable acceptance run interrupted; owned fixture cleanup finished.", file=sys.stderr)
        return 128 + interrupted.signum
    except KeyboardInterrupt:
        return 130
    except SystemExit:
        raise
    except BaseException:
        # pytest.fail is intentionally not an Exception. Its message may contain
        # container logs/admin secrets, so never render it or its traceback.
        print("Disposable acceptance setup failed; check source/image versions, fixture images and available ports.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(cli())
