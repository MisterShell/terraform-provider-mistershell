# Terraform Provider for MisterShell

The MisterShell Terraform provider enables infrastructure-as-code management of [MisterShell](https://www.mistershell.com) inventory — locations, network resources, and credentials.

## Requirements

- [Terraform](https://www.terraform.io/downloads) >= 1.5
- A running MisterShell instance
- A MisterShell API key (`yami_` prefixed)

## Installation

```hcl
terraform {
  required_providers {
    mistershell = {
      source  = "MisterShell/mistershell"
      version = "~> 0.1"
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
  credential_type = "ssh_password"

  credential_data = jsonencode({
    username = "admin"
    password = var.ssh_password
  })
}

# Add a network resource
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "FCW2345L0AB"
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
| `mistershell_resource` | `id`, `name`, `resource_type`, `location_id`, `status`, `health_status` |
| `mistershell_credential` | `id`, `name`, `credential_type` |

## Supported Resource Types

`cisco_ios`, `cisco_iosxe`, `cisco_iosxe_sdwan`, `cisco_nxos`, `cisco_ise`, `cisco_vbond`, `cisco_vmanage`, `cisco_vsmart`, `infoblox_nios`, `generic_ssh`, `linux`, `windows`, `panos_ssh`, `generic_rdp`, `aws_account`, `azure_subscription`, `kubernetes_cluster`, `database`

## Supported Credential Types

`ssh_password`, `ssh_key`, `aws_credentials`, `azure_service_principal`, `kubeconfig`, `rdp_password`, `db_password`

## Development

```bash
# Build
make build

# Run acceptance tests (requires a running MisterShell instance)
MISTERSHELL_URL=http://localhost:13000 MISTERSHELL_API_KEY=yami_xxx make test
```

### Generated type lists

The supported resource and credential type lists are **generated** from the
MisterShell backend's checked-in OpenAPI spec (`ui/openapi.json`,
`components.schemas.NetworkResourceType` and `components.schemas.CredentialType`)
so they never drift behind upstream. The output lives in
`internal/client/types_gen.go` (`SupportedResourceTypes` /
`SupportedCredentialTypes`), which is consumed by the `resource_type` and
`credential_type` validators.

```bash
# Regenerate (by default fetches the spec from the MisterShell git repo)
make generate
```

`internal/client/types_gen.go` is generated — **do not edit it by hand**.
Environment overrides for the generator:

| Variable | Default | Purpose |
|---|---|---|
| `MISTERSHELL_OPENAPI` | _(unset)_ | Path to a local `ui/openapi.json`; read directly instead of fetching from git (offline / CI path). |
| `MISTERSHELL_REPO` | `git@github.com:MisterShell/mistershell.git` | Git repo to fetch the spec from when `MISTERSHELL_OPENAPI` is unset. |
| `MISTERSHELL_REF` | `main` | Git ref/branch to fetch. |

```bash
# Regenerate offline from a local backend clone
MISTERSHELL_OPENAPI=/path/to/mistershell/ui/openapi.json make generate
```

## License

See [LICENSE](LICENSE).
