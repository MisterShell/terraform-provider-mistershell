# Changelog

Notable changes to the MisterShell Terraform provider, for people writing HCL against it.

This project adheres to [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.7] - 2026-09-11

**Requires MisterShell >= 2.0.7.**

### Changed

- Update `mistershell_role.permissions` HCL when upgrading from backend 2.0.1.
  The backend migrates stored role grants, but cannot edit Terraform configuration;
  retired names cause persistent drift and HTTP 400 `Invalid permission` on apply:

  | Previous permission | Use instead |
  |---|---|
  | `app.workers.read`, `app.proxies.read` | `app.fabric.read` |
  | `app.workers.write`, `app.proxies.write` | `app.fabric.write` |
  | `app.workers.delete`, `app.proxies.delete` | `app.fabric.delete` |
  | `app.resources.test`, `app.resources.session`, `app.snapshots.create` | `app.resources.execute` |
  | `app.ai.chat`, `app.ai.quick_assist` | `app.ai.execute` |
  | `app.facts.manage` | `app.facts.write` |
  | `app.changelog.write` | Remove it; use `app.changelog.delete` if deletion is required |

- `mistershell_setting` key `session_inactivity_timeout_minutes` now accepts
  1–240 minutes. The backend clamps larger stored values to 240; change HCL values
  above 240 to avoid repeated drift and HTTP 422 on apply.
- Log-destination creation and updates require a Pro license providing
  `log_forwarding.configure` and `app.audit.write`. Deletion requires
  `app.audit.delete`. Auth-provider mapping deletion requires
  `app.auth_providers.delete`. Ensure the Terraform API key retains the relevant
  delete grants; write access alone is insufficient for those destroy operations.
- Creating superuser roles (`*.*.*`) or mapping groups to them requires a
  superuser caller. The role name `admin` is reserved; built-in roles cannot be
  edited or deleted and should only be read through data sources.
- Clarified SSH trust management for MisterShell 2.0.7. Leave worker-observed
  `host_key_fingerprint` and `tls_fingerprint` out of `connector_data` during
  onboarding. Connector edits preserve an omitted saved SSH pin. Changing a pin
  or accepting a changed resource identity requires MisterShell's explicit
  check-and-accept workflow in the UI/API; this provider does not perform those
  acceptance operations.

### Fixed

- `mistershell_resource` with `is_enabled = false` now disables automatic snapshot
  collection in the initial create request. Previously, a follow-up update
  disabled it after creation, leaving a window in which collection could be
  scheduled. The existing attribute defaults to `true`; combine it with an
  in-place `location_id` update to park a resource. Manual operations remain
  available, and creation still requires live onboarding.
- Corrected the settings reference to list all 99 registered backend 2.0.7 keys,
  constraints, defaults, and secret flags. Removed nonexistent keys that fail
  with HTTP 404, and corrected the email-notification and API-rate-limit defaults.

## [2.0.1] - 2026-08-26

**Requires MisterShell >= 2.0.1.**

### Added

- `mistershell_resource` and its data source now expose the computed
  `effective_latitude` and `effective_longitude` inherited from the resource's
  location or its nearest ancestor with valid coordinates.
- The computed `coordinate_source_location_id` and
  `coordinate_source_location_name` attributes identify the location supplying
  those coordinates. They are null when that location is outside the caller's
  visible location scope, while the effective coordinates remain available.

## [2.0.0] - 2026-08-25

**Requires MisterShell >= 2.0.0.**

### Added

- Network-resource onboarding through a live connection. Set `engine_id` for
  automatic type discovery, or set `resource_type` to require one exact type.
  Automatic SSH onboarding falls back to `generic_ssh` after a successful
  connection when no more specific type matches. Explicit `generic_ssh`
  onboarding only requires a successful SSH connection.
- Native Terraform values for structured attributes such as `connector_data`,
  `credential_data`, resource configuration, schemas, and setting values.
- Runtime resource and credential type catalogs, including types supplied by
  installed extensions.
- Complete inventory, access-policy, external-authentication, worker, setting,
  and AI resource and data-source coverage for MisterShell 2.0.
- Resource discovery results including computed `resource_type`, nullable
  `external_id`, connection status, collection timestamps, and discovered
  metadata.
- Exact-one-match data-source lookup by ID or documented search filters, with
  complete pagination of collection endpoints.

## [0.1.7] - 2026-07-15

**Requires MisterShell >= 1.6.7.**

### Added

- `mistershell_auth_provider` and its data source: read-only `sp_entity_id` and `sp_acs_url`
  attributes for SAML — the service-provider values to register with your IdP. MisterShell
  derives them from the instance's `app_base_url` setting plus the provider ID. Both are null
  for LDAP and OIDC.

  ```hcl
  output "saml_acs_url" {
    value = mistershell_auth_provider.saml.sp_acs_url
  }
  ```

### Removed

- SAML `config` no longer accepts `sp_entity_id` / `sp_acs_url`. MisterShell now derives these
  values, and the SAML config rejects unknown keys, so supplying them fails the apply with
  `422 Extra inputs are not permitted`. Remove them from `config` and read the new attributes
  above instead:

  ```hcl
  config = jsonencode({
    idp_entity_id = "https://idp.example.com/metadata"
    idp_sso_url   = "https://idp.example.com/sso"
    idp_x509_cert = file("idp.crt")
    # sp_entity_id = "..."  <- remove
    # sp_acs_url   = "..."  <- remove
  })
  ```

### Fixed

- Documented the LDAP and OIDC `tls_verify` config field. The defaults differ: LDAP defaults to
  `false` (the connection is encrypted but the directory's certificate is **not** verified), while
  OIDC defaults to `true`. Set `tls_verify = true` on LDAP when your directory's certificate chains
  to a trusted CA.
- Documented that SAML `idp_x509_cert` must be a PEM certificate including the
  `BEGIN`/`END CERTIFICATE` markers; other values are rejected by the API.

## 0.1.6 and earlier

See the [GitHub releases](https://github.com/MisterShell/terraform-provider-mistershell/releases).
This changelog starts at 0.1.7.
