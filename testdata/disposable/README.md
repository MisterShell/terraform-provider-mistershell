# Disposable MisterShell acceptance tests

`run.py` owns a uniquely named Core, SSH target, HashiCorp Vault and OpenBao.
It uses the current MisterShell checkout's real API-container fixture, fresh
PostgreSQL/Redis state and signed test-only licensing. It never contacts a
configured development API. Fixture keys are temporary and removed on exit.

Prerequisites:

- Docker and the source checkout's Python test environment.
- A current MisterShell Core **test** image built with its test license key.
- The pinned vault images described in `../external-vaults/README.md`.
- Go, Terraform, `postgres:16-alpine` for the database identity fixture, and a built SSH fixture image:

```sh
docker build -t terraform-provider-mistershell-ssh-linux:latest testdata/ssh-linux
```

Run from the provider repository root, replacing checkout and image paths:

```sh
/path/to/mistershell/.venv/bin/python testdata/disposable/run.py \
  --source /path/to/mistershell \
  --image mistershell-core:tf-provider-compat \
  -- go test ./internal/provider -count=1 -parallel=1 -timeout=45m -v
```

The runner verifies that the image reports the checkout's project version.
Use `--api-port` and `--ssh-port` for otherwise unused ports; development ports
are rejected. `TF_ACC_TERRAFORM_PATH` can select a specific Terraform executable.
The current suite includes all resource families and their data sources, update
and clearing regressions, imports, readable drift, ownership boundaries,
explicit SSH trust acceptance and reset-on-destroy behavior. Backend assertions
verify persisted values and deletion/reset independently from Terraform state.
Platform-specific connector tests still require their documented Windows/network
fixtures; superseded legacy cases remain marked as such in the existing suite.

Shell profile and shell-profile mapping support is intentionally excluded.

The same runner is exposed by `make test-disposable`; override
`MISTERSHELL_SOURCE`, `MISTERSHELL_TEST_IMAGE`, or `MISTERSHELL_TEST_PYTHON` as needed.

To verify private failure diagnostics and cancellation cleanup against actual
containers, run the runner's own lifecycle tests:

```sh
DISPOSABLE_CORE_SOURCE=/path/to/mistershell \
DISPOSABLE_CORE_IMAGE=mistershell-core:tf-provider-compat \
  /path/to/mistershell/.venv/bin/python -m unittest discover \
  -s testdata/disposable -p test_runner.py -v
```

The runner handles SIGINT and SIGTERM by stopping the test process group and
removing its owned Core, SSH, vault and database fixtures. Bootstrap exceptions,
including pytest fixture failures, are redacted; private bootstrap logs are
removed when the invocation exits. A disabled backend-owned sensor index source
is seeded for import ownership tests and is never fetched.

Verify failure redaction and real signal cleanup with:

```sh
DISPOSABLE_CORE_SOURCE=/path/to/mistershell \
DISPOSABLE_CORE_IMAGE=mistershell-core:tf-provider-compat \
/path/to/mistershell/.venv/bin/python -m unittest discover \
  -s testdata/disposable -p test_runner.py -v
```
