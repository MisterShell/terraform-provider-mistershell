# Terraform Provider for MisterShell

The MisterShell Terraform provider enables infrastructure-as-code management of [MisterShell](https://www.mistershell.com) inventory — locations, network resources, and credentials.

## Requirements

- [Terraform](https://www.terraform.io/downloads) >= 1.5
- A running MisterShell 2.0 or newer instance
- A MisterShell API key (`apikey_` prefixed)

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

  credential_data = jsonencode({
    username = "admin"
    password = var.ssh_password
  })
}

# Onboard a network resource. MisterShell makes a live SSH connection,
# discovers the native type, and falls back to Generic SSH when none matches.
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  engine_id     = "ssh"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.1"
    port = 22
  })
}
```

## Resources

| Resource | Description |
|---|---|
| `mistershell_location` | Manages a location (geographic or organizational hierarchy) |
| `mistershell_resource` | Manages a network resource (device, cloud account, cluster) |
| `mistershell_credential` | Manages an encrypted credential (SSH, AWS, Azure, Kubeconfig) |

## Data Sources

All data sources support lookup by `id` or by search filters. Filters must match exactly one result.

| Data Source | Lookup by |
|---|---|
| `mistershell_location` | `id`, `name`, `kind`, `parent_id` |
| `mistershell_resource` | `id`, `name`, `resource_type`, `location_id`, `status` |
| `mistershell_credential` | `id`, `name`, `credential_type` |

## Runtime RDK catalogs

Resource and credential types come from the RDK bundles installed on the target
MisterShell instance, and extensions may add types. The provider reads
`/api/v1/resources/types` and `/api/v1/credentials/types` at runtime instead of
embedding closed lists.

## Development

```bash
# Build
make build

# Run acceptance tests (requires a running MisterShell instance)
MISTERSHELL_URL=http://localhost:13000 MISTERSHELL_API_KEY=apikey_xxx make test
```

### Generated enums

Stable non-RDK enums are generated from the MisterShell backend's checked-in
OpenAPI specification. RDK resource and credential catalogs are intentionally
runtime data and are not generated into the provider.

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
