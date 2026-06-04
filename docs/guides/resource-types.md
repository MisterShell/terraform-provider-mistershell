---
page_title: "Resource Types & connector_data"
subcategory: "Reference"
description: |-
  Every mistershell_resource resource_type, the connector_data fields it accepts, and the credential types it pairs with.
---

# Resource Types & connector_data

A `mistershell_resource` is described by its `resource_type`, which determines
the connector used to reach it, the fields accepted in `connector_data` (set
with `jsonencode(...)` in HCL), and which `credential_type`(s) it can pair with.

`connector_data` fields marked **required** must be present; optional fields
fall back to the noted defaults when omitted.

## Resource type reference

| resource_type | Connector | Required connector_data | Optional connector_data (default) | Compatible credential_type(s) |
|---|---|---|---|---|
| `cisco_ios` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_iosxe` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_iosxe_sdwan` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_ise` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_nxos` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_vbond` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_vmanage` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `cisco_vsmart` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `generic_ssh` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `infoblox_nios` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `linux` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `panos_ssh` | SSH | `host` | `port` (22) | `ssh_password`, `ssh_key` |
| `windows` | SSH/OpenSSH + RDP | `host` | `port` (22), `rdp_port` (3389), `nla_required` (true), `keyboard_layout` ("" = en-US) | `ssh_password`, `ssh_key`, `rdp_password` |
| `generic_rdp` | RDP | `host` | `rdp_port` (3389), `nla_required` (true), `keyboard_layout` ("" = en-US) | `rdp_password` |
| `database` | usql | `engine`, `host` | `port` (engine default; see below) | `db_password` |
| `aws_account` | AWS API | _(none)_ | `hostname` (account alias) | `aws_credentials` |
| `azure_subscription` | Azure API | `tenant_id`, `subscription_id` | `cloud` (AzureCloud) | `azure_service_principal` |
| `kubernetes_cluster` | Kubernetes API | _(none)_ | `context` (kubeconfig context), `namespace` (default namespace) | `kubeconfig` |

The `credential_type` ↔ `resource_type` pairing is enforced by the server: an
incompatible pairing is rejected with an HTTP 400.

## Field details

### SSH family

`cisco_ios`, `cisco_iosxe`, `cisco_iosxe_sdwan`, `cisco_ise`, `cisco_nxos`,
`cisco_vbond`, `cisco_vmanage`, `cisco_vsmart`, `generic_ssh`, `infoblox_nios`,
`linux`, `panos_ssh`.

- `host` (string, **required**) — hostname or IP.
- `port` (number, optional, default `22`).

```hcl
connector_data = jsonencode({
  host = "10.0.1.1"
  port = 22
})
```

### windows

Windows reaches the host over SSH/OpenSSH (for collection) and RDP (for
interactive sessions).

- `host` (string, **required**).
- `port` (number, optional, default `22`) — SSH/OpenSSH port.
- `rdp_port` (number, optional, default `3389`).
- `nla_required` (bool, optional, default `true`).
- `keyboard_layout` (string, optional) — e.g. `"0x0000040C"`; `""` = en-US.

```hcl
connector_data = jsonencode({
  host            = "10.0.2.10"
  port            = 22
  rdp_port        = 3389
  nla_required    = true
  keyboard_layout = "0x0000040C"
})
```

### generic_rdp

- `host` (string, **required**).
- `rdp_port` (number, optional, default `3389`).
- `nla_required` (bool, optional, default `true`).
- `keyboard_layout` (string, optional).

```hcl
connector_data = jsonencode({
  host         = "10.0.2.20"
  rdp_port     = 3389
  nla_required = true
})
```

### database

- `engine` (string, **required**) — one of `postgres`, `mysql`, `sqlserver`,
  `clickhouse`.
- `host` (string, **required**).
- `port` (number, optional) — when omitted, the engine default is used:

| engine | default port |
|---|---|
| `postgres` | 5432 |
| `mysql` | 3306 |
| `sqlserver` | 1433 |
| `clickhouse` | 9000 |

```hcl
connector_data = jsonencode({
  engine = "postgres"
  host   = "10.0.3.20"
  port   = 5432
})
```

### aws_account

- `hostname` (string, optional) — account alias. No required fields.

```hcl
connector_data = jsonencode({
  hostname = "prod-account"
})
```

### azure_subscription

- `tenant_id` (string, **required**).
- `subscription_id` (string, **required**).
- `cloud` (string, optional, default `AzureCloud`) — one of `AzureCloud`,
  `AzureChinaCloud`, `AzureUSGovernment`, `AzureGermanCloud`.

```hcl
connector_data = jsonencode({
  tenant_id       = "00000000-0000-0000-0000-000000000002"
  subscription_id = "00000000-0000-0000-0000-000000000003"
  cloud           = "AzureCloud"
})
```

### kubernetes_cluster

- `context` (string, optional) — kubeconfig context to use.
- `namespace` (string, optional) — default namespace. No required fields.

```hcl
connector_data = jsonencode({
  context   = "prod"
  namespace = "default"
})
```

See the [Credential Types & credential_data](./credential-types) guide for the
fields each `credential_type` accepts.
