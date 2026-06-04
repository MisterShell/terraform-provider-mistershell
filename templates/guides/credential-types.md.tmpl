---
page_title: "Credential Types & credential_data"
subcategory: "Reference"
description: |-
  Every mistershell_credential credential_type, the credential_data fields it accepts, which are secret, and the resource types that use them.
---

# Credential Types & credential_data

A `mistershell_credential` is described by its `credential_type`, which
determines the fields accepted in `credential_data` (set with `jsonencode(...)`
in HCL) and which `resource_type`(s) can reference it.

Fields marked **required** must be present. Fields marked **secret** are masked
by the API in responses: their values are stored from your configuration and
never read back from the server, so `credential_data` is managed entirely from
config. After importing a credential by ID you must set `credential_data` in
config yourself.

The `credential_type` ↔ `resource_type` pairing is enforced by the server: an
incompatible pairing is rejected with an HTTP 400.

## Credential type reference

| credential_type | Required fields | Optional fields | Secret (masked) fields | Used by resource_type(s) |
|---|---|---|---|---|
| `ssh_password` | `username`, `password` | `enable_password` | `password`, `enable_password` | all SSH-family, `windows` |
| `ssh_key` | `username`, `private_key` | `passphrase`, `enable_password` | `private_key`, `passphrase`, `enable_password` | all SSH-family, `windows` |
| `aws_credentials` | `access_key`, `secret_key` | _(none)_ | `secret_key` | `aws_account` |
| `azure_service_principal` | `client_id`, `client_secret` | _(none)_ | `client_secret` | `azure_subscription` |
| `kubeconfig` | `kubeconfig` | _(none)_ | `kubeconfig` | `kubernetes_cluster` |
| `rdp_password` | `username`, `password` | `domain` | `password` | `windows`, `generic_rdp` |
| `db_password` | `username`, `password` | _(none)_ | `password` | `database` |

The SSH-family resource types are: `cisco_ios`, `cisco_iosxe`,
`cisco_iosxe_sdwan`, `cisco_ise`, `cisco_nxos`, `cisco_vbond`, `cisco_vmanage`,
`cisco_vsmart`, `generic_ssh`, `infoblox_nios`, `linux`, `panos_ssh`.

## Field details

### ssh_password

- `username` (**required**).
- `password` (**required**, secret).
- `enable_password` (optional, secret) — privileged-mode/enable password.

```hcl
credential_data = jsonencode({
  username        = "admin"
  password        = var.ssh_password
  enable_password = var.enable_password
})
```

### ssh_key

- `username` (**required**).
- `private_key` (**required**, secret) — PEM-encoded key (multiline).
- `passphrase` (optional, secret).
- `enable_password` (optional, secret).

```hcl
credential_data = jsonencode({
  username    = "admin"
  private_key = var.ssh_private_key
  passphrase  = ""
})
```

### aws_credentials

- `access_key` (**required**).
- `secret_key` (**required**, secret).

```hcl
credential_data = jsonencode({
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
})
```

### azure_service_principal

- `client_id` (**required**).
- `client_secret` (**required**, secret).

```hcl
credential_data = jsonencode({
  client_id     = var.azure_client_id
  client_secret = var.azure_client_secret
})
```

### kubeconfig

- `kubeconfig` (**required**, secret) — full kubeconfig YAML (multiline).

```hcl
credential_data = jsonencode({
  kubeconfig = var.kubeconfig
})
```

### rdp_password

- `username` (**required**).
- `domain` (optional).
- `password` (**required**, secret).

```hcl
credential_data = jsonencode({
  username = "Administrator"
  domain   = "CORP"
  password = var.rdp_password
})
```

### db_password

- `username` (**required**).
- `password` (**required**, secret).

```hcl
credential_data = jsonencode({
  username = "app"
  password = var.db_password
})
```

See the [Resource Types & connector_data](./resource-types) guide for the
`connector_data` fields each `resource_type` accepts.
