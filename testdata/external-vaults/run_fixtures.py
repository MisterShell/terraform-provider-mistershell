#!/usr/bin/env python3
"""Hold isolated local Vault/OpenBao fixtures until SIGINT or SIGTERM."""

import argparse
import json
import os
from pathlib import Path
import secrets
import signal
import subprocess
import sys
import tempfile
import threading
import time
from urllib.error import HTTPError, URLError
from urllib.request import ProxyHandler, Request, build_opener


IMAGES = {
    "hashicorp": "hashicorp/vault:2.1.2@sha256:c2f666266f383d2cf424d86b8bb8ce7d065562173ffec2b476d762943608bb55",
    "openbao": "openbao/openbao:2.7.0@sha256:71156a1c6623a5fa3f5e61b0c6a8ead0faf0df29a778339188443551995d1315",
}
LABEL = "mistershell.terraform.vault-fixture"
HTTP = build_opener(ProxyHandler({}))


class FixtureError(Exception):
    """A diagnostic safe to print: never includes tokens or response bodies."""


def docker(*args):
    try:
        result = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=30)
    except (OSError, subprocess.TimeoutExpired):
        raise FixtureError("Docker is unavailable or did not respond within 30 seconds") from None
    if result.returncode:
        # Docker arguments and output can contain the root token; keep both private.
        raise FixtureError("Docker command failed; check Docker access, local pinned images and the optional network")
    return result.stdout.strip()


def request(address, path, token=None, body=None):
    headers = {"Content-Type": "application/json"}
    if token is not None:
        headers["X-Vault-Token"] = token
    req = Request(address + "/v1/" + path, headers=headers,
                  data=None if body is None else json.dumps(body).encode())
    try:
        with HTTP.open(req, timeout=5) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except HTTPError as error:
        raise FixtureError(f"Vault API returned HTTP {error.code} during fixture setup") from None
    except (URLError, TimeoutError, OSError):
        raise FixtureError("Local vault API is unavailable") from None


def start_fixture(kind, owner, network, stopped):
    name = f"tf-mistershell-vault-{kind}-{owner}"
    token = secrets.token_urlsafe(32)
    prefix = "VAULT" if kind == "hashicorp" else "BAO"
    args = ["run", "-d", "--pull=never", "--name", name,
            "--label", f"{LABEL}={owner}", "--cap-add=IPC_LOCK",
            "-p", "127.0.0.1::8200",
            "-e", f"{prefix}_DEV_ROOT_TOKEN_ID={token}",
            "-e", f"{prefix}_DEV_LISTEN_ADDRESS=0.0.0.0:8200"]
    if network:
        args.extend(["--network", network])
    args.extend([IMAGES[kind], "server", "-dev"])
    docker(*args)
    port = docker("port", name, "8200/tcp").rsplit(":", 1)[1]
    address = "http://127.0.0.1:" + port
    deadline = time.monotonic() + 30
    while True:
        if stopped.is_set():
            raise InterruptedError
        try:
            if request(address, "sys/health").get("sealed") is False:
                break
        except FixtureError:
            pass
        if time.monotonic() >= deadline:
            raise FixtureError(f"{kind} did not become ready within 30 seconds")
        stopped.wait(0.1)
    fixture = {"url": address,
               "backend_url": f"http://{name}:8200" if network else address,
               "token": token, "role_id": "", "secret_id": ""}
    if kind == "openbao":
        request(address, "sys/auth/approle", token, {"type": "approle"})
        request(address, "sys/policies/acl/terraform-reader", token, {
            "policy": 'path "secret/data/*" { capabilities = ["read"] }',
        })
        request(address, "auth/approle/role/terraform-reader", token, {
            "token_policies": ["terraform-reader"], "token_no_default_policy": True,
            "token_ttl": "300s", "token_max_ttl": "300s",
        })
        fixture["role_id"] = request(
            address, "auth/approle/role/terraform-reader/role-id", token)["data"]["role_id"]
        fixture["secret_id"] = request(
            address, "auth/approle/role/terraform-reader/secret-id", token, {})["data"]["secret_id"]
    return fixture


def publish(output, fixtures):
    """Publish complete mode-0600 JSON atomically without replacing any existing file."""
    fd, temporary = tempfile.mkstemp(prefix=".vault-fixtures-", dir=output.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            os.fchmod(stream.fileno(), 0o600)
            json.dump(fixtures, stream)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        # A hard link is an atomic create-only publication on the same filesystem.
        os.link(temporary, output)
        return os.stat(temporary).st_ino, os.stat(temporary).st_dev
    finally:
        os.unlink(temporary)


def cleanup(owner, output, metadata_identity):
    success = True
    try:
        containers = docker("ps", "-aq", "--filter", f"label={LABEL}={owner}").split()
        if containers:
            docker("rm", "-f", *containers)
    except FixtureError:
        print("Cleanup could not remove owned fixture containers; check Docker access.", file=sys.stderr)
        success = False
    if metadata_identity is not None:
        try:
            current = output.lstat()
            if (current.st_ino, current.st_dev) == metadata_identity:
                output.unlink()
        except FileNotFoundError:
            pass
        except OSError:
            print("Cleanup could not remove the owned secret metadata file.", file=sys.stderr)
            success = False
    return success


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--network", help="Existing Docker network shared with the isolated MisterShell backend")
    parser.add_argument("--output", required=True, type=Path, help="New JSON metadata file; parent directory must exist")
    args = parser.parse_args()
    output = args.output.absolute()
    if output.exists() or output.is_symlink() or not output.parent.is_dir():
        parser.error("--output must be a new file in an existing directory")
    stopped = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda _signal, _frame: stopped.set())
    owner = secrets.token_hex(12)
    metadata_identity = None
    result = 0
    try:
        # Fail before creating containers if a prerequisite is missing.
        for image in IMAGES.values():
            docker("image", "inspect", image)
        if args.network:
            docker("network", "inspect", args.network)
        fixtures = {}
        for kind in IMAGES:
            if stopped.is_set():
                raise InterruptedError
            fixtures[kind] = start_fixture(kind, owner, args.network, stopped)
        metadata_identity = publish(output, fixtures)
        print("Isolated vault fixtures ready; private metadata published. Stop with Ctrl-C or SIGTERM.", flush=True)
        stopped.wait()
    except InterruptedError:
        pass
    except FixtureError as error:
        print(str(error), file=sys.stderr)
        result = 1
    except Exception as error:
        # Avoid printing exception details that might include secret HTTP payloads.
        print(f"Fixture setup failed ({type(error).__name__}); owned resources will be cleaned up.", file=sys.stderr)
        result = 1
    finally:
        if not cleanup(owner, output, metadata_identity):
            result = 1
    return result


if __name__ == "__main__":
    sys.exit(main())
