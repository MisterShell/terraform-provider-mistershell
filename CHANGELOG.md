# Changelog

Notable changes to the MisterShell Terraform provider, for people writing HCL against it.

This project adheres to [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- API 2.0.22 resources and matching data sources for CA certificates, recording
  stores/rules, file stores/transfer rules, fact checks/policy rules, automation
  playbooks, Configure templates/stacks/policies/resource variables, proxies,
  sensors/custom sources/custom rules/alert rules, report templates, saved
  commands, and MOTDs. Shell profiles/mappings are intentionally excluded.
- Built-in AI agent and skill overrides, AI guardrail configuration, staged sensor
  rule overrides, auth-provider mapping lookup, and credential/vault/prompt
  variable catalogs.
- Resource health metadata, role builtin/scope metadata, writable inventory
  configuration metadata, and explicit verified trust/identity acceptance.
- Repeatable disposable Core, SSH, PostgreSQL and external-vault acceptance
  fixtures, including interruption cleanup and private startup diagnostics.
- `mistershell_external_vault` resource and data source for HashiCorp Vault,
  OpenBao, AWS Secrets Manager and Azure Key Vault. Supports imports, exact
  lookup, masked configuration and explicit resets of removed config keys.
- Independent platform/user external sources on `mistershell_credential` and
  its data source, with vault references and optional literal fields. Source
  changes preserve the credential ID. External resource onboarding passes the
  planned name for reference templates. Tested against MisterShell 2.0.22;
  existing local workflows retain their older-backend request shape.

### Fixed

- Preserve unmanaged tag assignments on metadata updates.
- Refresh readable JSON configuration and import metadata while retaining masked
  secrets; send explicit resets when optional configurations are removed.
- Apply role scope/permission transitions atomically; resolve users through detail
  endpoints; permit imported external users to leave passwords unmanaged.
- Bound exact-name search prefixes to the API limit, preserving Unicode and exact
  matching; validate canonical emails and organizational coordinates before apply.
- Permit supported credential, log-destination, and custom AI-agent type changes
  in place; support safe configuration of the default worker.
- Clarify that legacy ACL enabled state does not control policy evaluation.
- Revoke role permissions when their managed attribute is removed; preserve SAML
  certificate formatting when the backend only trims whitespace.
- Protect webhook secrets in automation graphs and preserve unknown imported
  vault secrets through unrelated updates.
- Require explicit import before adopting existing sensor overrides or saved
  resource variables; reject managed imports of backend-owned catalog objects.
- Validate CA certificates again after unknown values resolve, refuse automatic
  ruleset publication, and reject unsupported sensor parameter clearing.
- Honor `MISTERSHELL_INSECURE=1` or `true` (case-insensitive) when the provider's
  `insecure` attribute is omitted. Explicit HCL values take precedence. The
  acceptance-test license probe now uses the same configured TLS client.
- Set and restore `app_base_url` in OIDC/SAML acceptance tests so they work on
  a fresh instance. Auth-provider examples now configure this prerequisite and
  declare the dependency before enabling OIDC/SAML.

## [2.0.15] - 2026-09-24

**Requires MisterShell >= 2.0.7.** Adds support for MisterShell 2.0.15; each
feature below states the backend release it needs.

### Added

- `mistershell_resource.sshgw_allow_personal_key` (optional boolean, defaults
  to `false`): when `true`, a user's own SSH key, registered in MisterShell
  under My Account → Account Security → My SSH keys, is accepted on its own in
  place of the account password and the second authentication factor (MFA)
  for direct native SSH sessions to that resource through the MisterShell SSH
  gateway, so non-interactive automation can connect. The account must still
  be active, and the resource credential, session policy, session recording,
  and audit logging still apply. Only resource types with a shell session mode
  (SSH, database, AWS, Azure, Kubernetes) accept `true`: setting it on
  `generic_rdp`, `generic_vnc`, or `generic_web` fails at apply with HTTP 400.
  The `mistershell_resource` data source reads it back.
  **Requires MisterShell >= 2.0.15.** Older backends reject any create or
  update that sends it with HTTP 422 and read it back as `false`; the provider
  sends it only when it is `true` on create or when it changes, so
  configurations that leave it at the default keep working on older backends.
  MisterShell 2.0.15 removed API-key sign-in over SSH and does not carry
  existing opt-ins over: after upgrading the backend, set
  `sshgw_allow_personal_key = true` on each resource that needs it and have the
  automation users register an SSH key.
- `mistershell_session_policy_rule` accepts `action = "approve"` with a new
  `approval` block (`approver_role_ids`, `expiration_seconds` default `86400`,
  `validity_seconds` default `3600`, `notify_email` default `false`,
  `allow_self_approval` default `false`). A matching user must get a decision
  from a member of an approver role before opening a new connection; approve
  rules are evaluated before all accept/deny rules regardless of `position`.
  The `approval` block is required for `approve` and not allowed for `accept`
  or `deny`, and approve rules cannot set `command_acl_ids`; the provider
  reports these mistakes at plan time. The `mistershell_session_policy_rule`
  data source returns `approval` (null for accept/deny rules).
  **Requires MisterShell >= 2.0.10.** Older backends reject `approve` with
  HTTP 422; accept/deny rules keep working on them.
- LDAP `config.mfa_method` is now documented and validated: `none` (the
  default on create) or `totp` (authenticator app). **Requires MisterShell >=
  2.0.12.** Email MFA (`mfa_method = "email"`) is not supported by this
  provider, because MisterShell enables it only after an interactive
  emailed-code check; the provider rejects it at plan time. To use email MFA,
  enable it in the MisterShell UI and omit `mfa_method` from `config`, which
  leaves the stored method unchanged on update. MisterShell 2.0.9–2.0.11 used
  `email_mfa_enabled` instead; 2.0.12 and later reject that key with HTTP 422,
  so remove it from `config`.
- Read-only `auth_provider_id` on `mistershell_user`, `mistershell_user`
  (data source), and each `mistershell_users` element: the external
  authentication provider a user signs in through, null for `LOCAL` users and
  on MisterShell < 2.0.9.
- The `mistershell_setting` pages list the 11 settings added through
  MisterShell 2.0.15: `carto_api_key` (secret), `max_shell_sessions_per_resource`,
  `sshgw_enabled`, `sshgw_port`, `sshgw_host_key` (secret),
  `sshgw_internal_token` (secret), `sshgw_max_connections`,
  `sshgw_unauth_connections_per_ip`, `sshgw_auth_attempts_per_ip_per_10min`,
  `sshgw_max_tabs`, and `local_mfa`. `local_mfa` can be read but not managed:
  MisterShell rejects writes and resets of it with HTTP 400 `Manage Local MFA
  through Auth Providers`.

### Changed

- Update `mistershell_role.permissions` in HCL when upgrading the backend from
  2.0.7 to 2.0.15. MisterShell migrates stored grants, and HCL that is not
  updated either fails or plans to remove the migrated grants:

  | Previous grant | Change in HCL |
  |---|---|
  | `app.tasks.delete` | Remove it (scheduled tasks can no longer be deleted); keeping it fails with HTTP 400 `Invalid permission` |
  | `app.collector.read` | Add `app.collector.execute` to keep viewing collected syslog messages |
  | `app.sensors.read` | Add `app.sensors.execute` to keep viewing IDS alerts |
  | `app.fact_policies.read` | Add `app.fact_policies.execute` to keep viewing compliance results and history |

  The backend adds each `*.execute` grant to every role that held the matching
  `*.read` grant. If HCL is not updated, the next apply removes it and the role
  loses that visibility.

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
