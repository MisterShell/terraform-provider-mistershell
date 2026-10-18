# Local external-vault acceptance fixtures

`run_fixtures.py` starts isolated HashiCorp Vault and OpenBao dev servers using
Python 3's standard library and Docker. It needs no MisterShell source checkout
or Python packages. Each container has a unique name, a random root token, and
an ephemeral port bound to `127.0.0.1`. The OpenBao AppRole has read-only access
to `secret/data/*`; its tokens carry no default policy.

These fixtures intentionally create containers, vault authentication settings,
policies, and test secrets. Use an isolated acceptance backend, never production.
The acceptance test also creates and deletes MisterShell resources and credentials.
Dev servers use HTTP and ephemeral storage and are only intended for these tests.

## Start the fixtures

Make these pinned images available locally first (the runner uses `--pull=never`):

```sh
docker pull hashicorp/vault:2.1.2@sha256:c2f666266f383d2cf424d86b8bb8ce7d065562173ffec2b476d762943608bb55
docker pull openbao/openbao:2.7.0@sha256:71156a1c6623a5fa3f5e61b0c6a8ead0faf0df29a778339188443551995d1315
```

From the repository root, start the runner in a terminal and leave it running:

```sh
python3 testdata/external-vaults/run_fixtures.py \
  --network mistershell_test_network \
  --output /tmp/mistershell-vault-fixtures.json
```

`--network` must identify an existing Docker network shared with the isolated
MisterShell backend. It makes `backend_url` use container DNS while `url` remains
the localhost address used by the Go tests to seed secrets. Omit `--network`
when Core runs on the host and can use the localhost address. A containerized
Core cannot use the host's `127.0.0.1` address; provide its shared network.

The output file must not already exist, and its parent directory must exist.
The runner publishes complete JSON atomically with permissions `0600` and never
prints credentials. The JSON contains `hashicorp` and `openbao` entries, each
with `url`, `backend_url`, `token`, `role_id`, and `secret_id`. HashiCorp uses
its root token for this fixture, so its AppRole fields are empty. The file and
container environments contain disposable secrets: do not commit or share them.

## Run vault integration acceptance

In another terminal, set credentials for an isolated, fully upgraded MisterShell
backend with the external-vault capability (Pro or Enterprise). The backend must
also satisfy the repository's SSH acceptance fixture prerequisites. Export its
`MISTERSHELL_URL` and `MISTERSHELL_API_KEY`; use `MISTERSHELL_INSECURE=true` only
if that test backend uses a self-signed certificate. Then run:

```sh
MISTERSHELL_ACC_VAULT_FIXTURES=/tmp/mistershell-vault-fixtures.json \
  TF_ACC=1 go test ./internal/provider \
  -run '^TestAccExternalVault_(resourceOnboarding|userIdentity)$' -count=1 -v
```

The test seeds a dedicated KV v2 secret, onboards an SSH resource through Core,
and cleans up its secret. HashiCorp uses token authentication; OpenBao uses the
read-only AppRole. This checks real secret resolution with resource-name
reference templates. The identity test creates a temporary JWT auth mount using
public keys from the live backend, verifies subject-specific access and denial
of another user's path, and restores `app_base_url` afterward. Run these tests
sequentially against the isolated instance. Cloud authentication and dynamic
JWKS retrieval/rotation are not qualified by these fixtures.

Stop the runner with Ctrl-C or send SIGTERM when tests finish. It removes only
containers carrying that run's unique ownership label and deletes its own
metadata file. It never deletes the supplied Docker network. Forced termination
with SIGKILL cannot run cleanup; inspect containers labeled
`mistershell.terraform.vault-fixture` and remove only those from the abandoned
run, along with that run's metadata file.

## Verify the runner itself

These tests start additional uniquely named containers and a temporary Docker
network. They verify localhost and Docker URLs, real AppRole login/read access,
write denial, private metadata permissions, existing-file protection, and
SIGINT/SIGTERM cleanup:

```sh
python3 -m unittest discover -s testdata/external-vaults -p test_fixture_runner.py
```
