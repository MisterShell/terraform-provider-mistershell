---
page_title: "mistershell_resource Resource - terraform-provider-mistershell"
subcategory: ""
description: |-
  Manages a MisterShell network resource (device, cloud account, Kubernetes cluster).
---

# mistershell_resource (Resource)

Manages a MisterShell network resource (device, cloud account, Kubernetes cluster).

## Example Usage

```terraform
# SSH device (Cisco IOS-XE switch) -> ssh_password / ssh_key
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "FCW2345L0AB"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.1"
    port = 22 # optional, default 22
  })
}

# Windows server (SSH/OpenSSH + RDP) -> ssh_password (also accepts ssh_key, rdp_password)
resource "mistershell_resource" "win_jumpbox" {
  name          = "win-jump-01.zurich"
  resource_type = "windows"
  external_id   = "WIN-JUMP-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host            = "10.0.2.10"
    port            = 22           # SSH/OpenSSH port, default 22
    rdp_port        = 3389         # default 3389
    nla_required    = true         # default true
    keyboard_layout = "0x0000040C" # "" = en-US
  })
}

# RDP-only host -> rdp_password
resource "mistershell_resource" "rdp_host" {
  name          = "rdp-app-01.zurich"
  resource_type = "generic_rdp"
  external_id   = "RDP-APP-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.rdp_admin.id

  connector_data = jsonencode({
    host         = "10.0.2.20"
    rdp_port     = 3389 # default 3389
    nla_required = true # default true
  })
}

# SQL database -> db_password
resource "mistershell_resource" "app_db" {
  name          = "app-postgres-01.zurich"
  resource_type = "database"
  external_id   = "app-postgres-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.app_db.id

  connector_data = jsonencode({
    engine = "postgres" # postgres | mysql | sqlserver | clickhouse
    host   = "10.0.3.20"
    port   = 5432 # optional, engine default (postgres 5432)
  })
}

# AWS account -> aws_credentials
resource "mistershell_resource" "aws_account" {
  name          = "aws-production"
  resource_type = "aws_account"
  external_id   = "123456789012"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.aws_prod.id

  connector_data = jsonencode({
    hostname = "prod-account" # optional account alias
  })
}

# Azure subscription -> azure_service_principal
resource "mistershell_resource" "azure_subscription" {
  name          = "azure-production"
  resource_type = "azure_subscription"
  external_id   = "00000000-0000-0000-0000-000000000003"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.azure_prod.id

  connector_data = jsonencode({
    tenant_id       = "00000000-0000-0000-0000-000000000002"
    subscription_id = "00000000-0000-0000-0000-000000000003"
    cloud           = "AzureCloud" # default AzureCloud
  })
}

# Kubernetes cluster -> kubeconfig
resource "mistershell_resource" "k8s_cluster" {
  name          = "prod-cluster"
  resource_type = "kubernetes_cluster"
  external_id   = "prod-cluster"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.k8s.id

  connector_data = jsonencode({
    context   = "prod" # optional kubeconfig context
    namespace = "default"
  })
}

# Tagging a resource from the resource side.
#
# When `tag_ids` is set, the provider manages the resource's tags EXCLUSIVELY:
# on every apply it adds/removes tags so the live set equals `tag_ids` exactly.
# Reference each tag by id (this also creates the proper dependency ordering).
#
# Own each tag<->resource edge from ONE side only: set `tag_ids` here OR list the
# resource in `mistershell_tag.resource_ids`, never both.
#   - Omit `tag_ids` entirely (null) to leave tags UNMANAGED (manage them from
#     the tag side instead); the live tags are still read back into `tags`.
#   - Set `tag_ids = []` to keep the resource exclusively tag-free.
resource "mistershell_tag" "prod" {
  name  = "production"
  color = "#e53935"
}

resource "mistershell_tag" "zurich" {
  name  = "zurich"
  color = "blue"
}

resource "mistershell_resource" "tagged_switch" {
  name          = "edge-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "FCW2345L0CD"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.2"
  })

  # Exclusively manage this resource's tags from the resource side.
  tag_ids = [
    mistershell_tag.prod.id,
    mistershell_tag.zurich.id,
  ]
}

# `tags` is computed (read-back) and always reflects the live tags on the
# resource, whether or not `tag_ids` is managed.
output "tagged_switch_tags" {
  value = mistershell_resource.tagged_switch.tags
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `external_id` (String) Unique external identifier for the resource (hostname, account ID, etc.).
- `location_id` (Number) Location ID where this resource resides.
- `name` (String) Resource display name.
- `resource_type` (String) What the resource is (e.g. cisco_iosxe, linux, aws_account). Cannot be changed after creation.

### Optional

- `connector_data` (String) Type-specific connection parameters as JSON. Use jsonencode() in HCL. Fields vary by resource_type (e.g. host/port for SSH types, rdp_port/nla_required for windows and generic_rdp, engine/host/port for database); see the valid resource_type values table on this page for the fields per type.
- `credential_id` (Number) Credential ID for connecting to this resource.
- `is_enabled` (Boolean) Whether the resource is enabled for operations.
- `tag_ids` (Set of Number) Set of tag IDs to assign to this resource (reference them as mistershell_tag.<name>.id). When set, the provider manages the resource's tags **exclusively** (it adds and removes tags to match this set exactly; `[]` clears all tags). When **omitted/null**, the provider does not manage tags at all — leave it unset if you assign this resource's tags from the tag side via mistershell_tag.resource_ids. Own each tag↔resource edge from one side only.

### Read-Only

- `connector_id` (String) Connector type, derived by the server from resource_type.
- `created_at` (String) Creation timestamp.
- `extra_data` (String) Discovered metadata as JSON. Auto-populated by MisterShell from connectivity checks.
- `health_status` (String) Health status (healthy, degraded, critical, unknown).
- `id` (Number) Resource ID.
- `last_collection_at` (String) Timestamp of last data collection.
- `last_connectivity_check` (String) Timestamp of last connectivity check.
- `last_health_at` (String) Timestamp of last health check.
- `last_snapshot_at` (String) Timestamp of last configuration snapshot.
- `next_collection_at` (String) Scheduled time for next data collection.
- `status` (String) Connectivity status (unknown, verified, unreachable, auth_failed, error, identity_mismatch, snapshot_truncated).
- `tags` (Attributes List) The tags currently assigned to this resource (read-back), as objects with id, name, color, and description. Always reflects server state regardless of whether tag_ids is managed. (see [below for nested schema](#nestedatt--tags))
- `updated_at` (String) Last update timestamp.

<a id="nestedatt--tags"></a>
### Nested Schema for `tags`

Read-Only:

- `color` (String) Tag color.
- `description` (String) Tag description.
- `id` (Number) Tag ID.
- `name` (String) Tag name.

## Valid `resource_type` values

The `resource_type` determines the connector used to reach the resource, the
fields accepted in `connector_data` (set with `jsonencode(...)` in HCL), and
which `credential_type`(s) it can pair with. `credential_id` must reference a
credential whose type is compatible per the table below; an incompatible
pairing is rejected by the API. Required `connector_data` fields must be
present; optional fields fall back to the noted defaults when omitted.

| resource_type | Connector | Required connector_data | Optional connector_data (default) | Compatible credential_type(s) |
|---|---|---|---|---|
| `cisco_ios` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_iosxe` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_iosxe_sdwan` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_nxos` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_ise` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_vbond` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_vmanage` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_vsmart` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `generic_ssh` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `infoblox_nios` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `linux` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `panos_ssh` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `windows` | SSH/OpenSSH + RDP | `host` | `port` (22), `rdp_port` (3389), `nla_required` (true), `keyboard_layout` ("" = en-US) | `ssh_password`, `ssh_key`, `rdp_password` |
| `generic_rdp` | RDP | `host` | `rdp_port` (3389), `nla_required` (true), `keyboard_layout` ("" = en-US) | `rdp_password` |
| `database` | DB | `engine`, `host` | `port` (engine default; see below) | `db_password` |
| `aws_account` | AWS API | _(none)_ | `hostname` (account alias) | `aws_credentials` |
| `azure_subscription` | Azure API | `tenant_id`, `subscription_id` | `cloud` (AzureCloud) | `azure_service_principal` |
| `kubernetes_cluster` | Kubernetes API | _(none)_ | `context` (kubeconfig context), `namespace` (default namespace) | `kubeconfig` |

### database `engine` values

For `resource_type = "database"`, the `engine` field is required. When `port`
is omitted, the engine default is used:

| engine | default port |
|---|---|
| `postgres` | 5432 |
| `mysql` | 3306 |
| `sqlserver` | 1433 |
| `clickhouse` | 9000 |

### azure `cloud` values

For `resource_type = "azure_subscription"`, the optional `cloud` field accepts:

| cloud | notes |
|---|---|
| `AzureCloud` | default |
| `AzureChinaCloud` | |
| `AzureUSGovernment` | |
| `AzureGermanCloud` | |

## Read-only status values

`status` and `health_status` are computed by the server and reported back on the
resource.

### `status` values

| status | meaning |
|---|---|
| `unknown` | not yet evaluated |
| `verified` | reachable and authenticated |
| `unreachable` | host could not be reached |
| `auth_failed` | reachable but authentication failed |
| `error` | a collection or connectivity error occurred |
| `identity_mismatch` | the device identity did not match expectations |
| `snapshot_truncated` | the configuration snapshot was truncated |

### `health_status` values

| health_status | meaning |
|---|---|
| `healthy` | operating normally |
| `degraded` | partially impaired |
| `critical` | severely impaired |
| `unknown` | not yet evaluated |

## Tagging

A resource can carry zero or more tags ([`mistershell_tag`](tag.md)). This
resource exposes two tag attributes: `tag_ids` (input, optional) and `tags`
(computed, read-back).

### `tag_ids` — resource-side, exclusive management

| attribute | constraints |
|---|---|
| `tag_ids` | Optional. Set of tag IDs (`int64`). Order-independent. |

`tag_ids` is the **complete set** of tags the resource carries, managed from the
**resource side** via the resource-centric assignment endpoint. On every apply
the provider adds and removes tags so the live set equals `tag_ids` **exactly** —
tags assigned out of band (through the UI or another caller) are removed on the
next apply, and tags you drop from the set are unassigned.

Reference each tag by its **id**, e.g.:

```hcl
tag_ids = [
  mistershell_tag.prod.id,
  mistershell_tag.zurich.id,
]
```

Referencing by id (rather than a literal integer) also establishes the correct
Terraform dependency so the tags are created before they are assigned.

**`null` (omitted) vs. `[]` (empty set)** — these are different:

| value | behavior |
|---|---|
| omitted / `null` | **Unmanaged (hands-off).** The provider does not add, remove, or assert tags at all. The live tags are still read back into `tags` (see below). Use this when you manage the tag↔resource edges from the tag side instead. |
| `[]` (empty set) | **Exclusively empty.** The provider actively clears **all** tags from the resource and keeps it tag-free on every apply. |

**Exclusivity — own each edge from one side only.** A tag↔resource assignment
can be expressed from either side: from the resource via `tag_ids`, or from the
tag via [`mistershell_tag`](tag.md)'s `resource_ids`. Both attributes use
whole-set replace semantics and take exclusive ownership of their side, so
managing the **same** edge from both sides makes the two resources fight on every
plan (each apply reverts the other), producing perpetual diffs. **Pick one side
per edge** — set `tag_ids` here **or** list this resource in the tag's
`resource_ids`, never both. Leaving `tag_ids` omitted (null) is what makes
tag-side management safe.

### `tags` — computed read-back

`tags` is a **computed** list reflecting the live tags currently on the resource,
read back from the server on every refresh. It is always populated from server
state **regardless of whether `tag_ids` is managed** — including when `tag_ids`
is omitted and the edges are managed from the tag side. Each element is an object:

| field | description |
|---|---|
| `id` | Tag ID (`int64`). |
| `name` | Tag name. |
| `color` | Tag color (free-form string; see the [`mistershell_tag`](tag.md) page). |
| `description` | Tag description (may be null). |

## Import

Import is supported using the following syntax:

```shell
# Network resources are imported by their integer ID.
terraform import mistershell_resource.example 456
```
