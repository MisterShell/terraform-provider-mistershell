# Terraform Provider for MisterShell

The MisterShell Terraform provider enables infrastructure-as-code management of [MisterShell](https://www.mistershell.com) inventory, authentication, policies, external vaults, automation, configuration, sensors, proxies, reporting, and AI settings.

## Requirements

- [Terraform](https://www.terraform.io/downloads) >= 1.5
- A running MisterShell 2.0.7 or newer instance; some features need a newer backend (external vault support is tested on 2.0.22), as stated in the [CHANGELOG](CHANGELOG.md)
- A MisterShell API key (`apikey_` prefixed)

For repeatable end-to-end verification, use `make test-disposable`; setup and
fixture requirements are in [the disposable test guide](testdata/disposable/README.md).

## Current API coverage

The unreleased API-completeness changes target MisterShell 2.0.22. New resources
cover CA certificates, recording and file-transfer policies/stores, fact policies,
automation playbooks, Configure templates/stacks/policies/resource variables,
proxies, sensors and rule configuration, report templates, saved commands, MOTDs,
and built-in AI overrides/guardrails. Each has a matching lookup data source where
the API supports reads. Shell profiles and their mappings are excluded.

Type catalogs expose installed credential and external-vault JSON schemas.
See the generated [resource documentation](docs/resources) and
[data-source documentation](docs/data-sources) for fields and examples.

Sensitive attributes, including enrollment tokens and credential material, are
stored in Terraform state. API-masked secrets cannot be recovered on import or
checked for remote changes. Readable managed fields are refreshed for drift.
Built-in AI overrides restore owned settings on destroy; configuration resource
variables reset to an empty values object because the API has no delete operation.
Sensor rule overrides manage staged state; applying a shared ruleset remains an
explicit administrative operation.
Custom-rule writes require manual ruleset mode because automatic mode can publish
other operators' staged changes. The default worker remains in place on destroy,
including its configuration, as documented in its resource lifecycle.

## Installation

```hcl
terraform {
  required_providers {
    mistershell = {
      source  = "MisterShell/mistershell"
      version = "~> 2.0"
    }
  }
}
```

## Authentication

The provider authenticates using a MisterShell API key. Generate one from the MisterShell UI under your user profile.

```hcl
provider "mistershell" {
  url      = "https://mistershell.example.com"  # or MISTERSHELL_URL env var
  api_key  = var.mistershell_api_key             # or MISTERSHELL_API_KEY env var
  insecure = true                                # optional: skip TLS verification
}
```

`MISTERSHELL_INSECURE=1` or `true` (case-insensitive) also skips TLS verification.
An explicit `insecure = true` or `false` in HCL takes precedence; otherwise TLS
verification is enabled by default.

The API key's role needs read, write, and the relevant delete permissions for
the objects Terraform manages. Workers use `app.fabric.read`, `app.fabric.write`,
and `app.fabric.delete`;
log-destination deletion uses `app.audit.delete`, and auth-provider mapping
deletion uses `app.auth_providers.delete`. Log-destination creation/updates also
require a Pro license with `log_forwarding.configure`. See the [2.0.7](CHANGELOG.md#207---2026-09-11) and
[2.0.15](CHANGELOG.md#2015---2026-09-24) upgrade notes for renamed role
permissions, setting limits, and per-feature backend requirements.

Keep the API key's location scope covering all managed objects and their target
locations. An object outside that scope can return HTTP 404; Terraform treats it
as missing and may plan to recreate it. A global location scope is appropriate
when Terraform manages inventory across the entire instance.

## External vaults

Manage vault connections with `mistershell_external_vault` and select existing
connections with its data source. Credentials support independent platform and
user sources, including mixed literal values and vault references. See the
[vault resource](docs/resources/external_vault.md) and
[credential resource](docs/resources/credential.md) for examples and transitions.
Vault operations need the Pro/Enterprise external-vault capability and
`app.auth_providers.read/write/delete` permissions. Terraform stores connection
secrets and configured literals in state, but never reads resolved vault secrets.

## Example Usage

```hcl
# Build a location hierarchy
resource "mistershell_location" "emea" {
  name        = "EMEA"
  kind        = "geo"
  description = "Europe, Middle East, and Africa"
  parent_id  = 1
}

resource "mistershell_location" "zurich" {
  name        = "Zurich DC"
  kind        = "geo"
  description = "Zurich data center"
  parent_id   = mistershell_location.emea.id
  latitude    = 47.3769
  longitude   = 8.5417
}

# Create a credential
resource "mistershell_credential" "ssh_admin" {
  name            = "dc-admin-ssh"
  credential_type = "username_password"

  credential_data = {
    username = "admin"
    password = var.ssh_password
  }
}

# Onboard a network resource. MisterShell makes a live SSH connection,
# discovers the native type, and falls back to Generic SSH when none matches.
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  engine_id     = "ssh"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = {
    host = "10.0.1.1"
    port = 22
  }
}
```

## Resources

| Resource | Description |
|---|---|
| `mistershell_location` | Manages a location (geographic or organizational hierarchy) |
| `mistershell_resource` | Manages a network resource (device, cloud account, cluster) |
| `mistershell_credential` | Manages an encrypted credential (SSH, AWS, Azure, Kubeconfig) |

## Data Sources

Entity data sources support the identity or exact-match filters documented for
each type. Catalog data sources return installed types or available variables.

| Data Source | Lookup by |
|---|---|
| `mistershell_location` | `id`, `name`, `kind`, `parent_id` |
| `mistershell_resource` | `id`, `name`, `resource_type`, `location_id`, `status` |
| `mistershell_credential` | `id`, `name`, `credential_type` |

## Runtime type catalogs

Resource and credential types come from the target MisterShell instance, and
installed extensions may add types. The provider reads
`/api/v1/resources/types` and `/api/v1/credentials/types` at runtime instead of
embedding closed lists.

## Development

```bash
# Build
make build

# Run acceptance tests (requires a running MisterShell instance)
MISTERSHELL_URL=http://localhost:13000 MISTERSHELL_API_KEY=apikey_xxx make test
```

### Live acceptance suite

The acceptance suite creates, changes, imports, and removes live MisterShell
objects. Use an isolated development instance. Put `MISTERSHELL_URL`,
`MISTERSHELL_API_KEY`, and optional `MISTERSHELL_INSECURE` values in a gitignored
`.env` file, then start the deterministic Linux SSH discovery target:

```bash
make acceptance-fixture-up acceptance-fixture-wait
make test-live-staged
make sweep
make acceptance-fixture-down
```

`test-live-staged` runs onboarding, core inventory, policy/auth, AI, and edge-case
stages sequentially with Terraform parallelism disabled. The onboarding stage
also starts an in-process unknown SSH server to prove automatic Generic SSH
fallback. The Docker fixture is used to prove Linux discovery and explicit-type
onboarding.

OIDC/SAML acceptance tests temporarily set `app_base_url` to `MISTERSHELL_URL`
and restore its previous value after each subtest. This is an instance-wide
setting, so run the suite sequentially against the isolated development instance.

The defaults are `127.0.0.1:12223`, user `tfacc`, and password
`tfacc-password`. Override them with `MISTERSHELL_ACC_SSH_HOST`,
`MISTERSHELL_ACC_SSH_PORT`, `MISTERSHELL_ACC_SSH_USERNAME`, and
`MISTERSHELL_ACC_SSH_PASSWORD`; use `MISTERSHELL_ACC_SSH_UPDATE_HOST` for an
equivalent hostname used to verify connector-data updates. Values in `.env`
are honored by every staged target. To load credentials from a different file, set
`MISTERSHELL_ENV_FILE=/absolute/path/to/.env`.

For a MisterShell worker running on another host, expose the in-process Generic
SSH fixture with `MISTERSHELL_ACC_GENERIC_SSH_BIND_ADDRESS=0.0.0.0:<port>` and
set `MISTERSHELL_ACC_GENERIC_SSH_HOST` to an address reachable by that worker.

Run `make sweep` before and after interrupted suites. Sweepers only target the
per-run `tfacc-` namespace and delete dependent objects before their parents.

### Generated enums

Stable API enums are generated from the MisterShell backend's checked-in OpenAPI
specification. Resource and credential types remain runtime data so the provider
automatically supports the types installed on each MisterShell instance.

```bash
# Regenerate stable enums from a local checkout of the MisterShell backend
MISTERSHELL_OPENAPI=/path/to/mistershell/ui/openapi.json make generate
```

`internal/client/types_gen.go` is generated — **do not edit it by hand**. The
MisterShell backend is a private repository, so the generator has no built-in
git source; point it at a local `ui/openapi.json`. Generator environment
variables:

| Variable | Default | Purpose |
|---|---|---|
| `MISTERSHELL_OPENAPI` | _(unset)_ | Path to a local `ui/openapi.json` (from a backend checkout). Primary, recommended source. |
| `MISTERSHELL_REPO` | _(unset)_ | Optional git repo to fetch the spec from (git mode). No default — set this only if you have access to the private backend repo and prefer git over `MISTERSHELL_OPENAPI`. |
| `MISTERSHELL_REF` | `main` | Git ref/branch to fetch (git mode only). |

## License

See [LICENSE](LICENSE).

### External vault acceptance tests

`make test-live-vaults` covers vault lifecycle and credential sources. To also
exercise actual HashiCorp/OpenBao resolution and per-user JWT identity, start the
isolated fixtures described in [testdata/external-vaults](testdata/external-vaults/README.md)
and set `MISTERSHELL_ACC_VAULT_FIXTURES` to their JSON metadata file. The fixtures
must be reachable from MisterShell Core as well as the test runner. These tests
seed and remove test secrets and temporarily configure `app_base_url`; use an
isolated backend and run live suites sequentially. Cloud metadata/schema behavior
is tested for AWS/Azure; real cloud authentication needs separate accounts.
