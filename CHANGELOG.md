# Changelog

Notable changes to the MisterShell Terraform provider, for people writing HCL against it.

This project adheres to [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - Unreleased

**Requires MisterShell >= 2.0.0.** This release intentionally drops the
pre-2.0 provider contract; no backward-compatibility shim is provided.

### Added

- Live network-resource onboarding. Configure `engine_id` for automatic RDK
  discovery or `resource_type` for exact discovery. SSH automatically falls
  back to `generic_ssh` after a successful connection when no native detector
  matches; explicitly selecting `generic_ssh` only verifies the SSH connection.
- Discovery-task and proof polling with cancellation, terminal-state handling,
  a five-minute create limit, and proof-backed resource creation.
- Runtime resource and credential catalogs sourced from the MisterShell/RDK API,
  including installed extension types.
- MisterShell 2.0 fields including role `location_scope_mode`, auth-provider
  `mnemonic` and `oidc_redirect_uri`, worker RDK compatibility metadata, setting
  schema/encryption metadata, AI-tool agent-type restrictions, user permissions,
  permission-role assignments, and location path/resource counts.
- Metadata-driven pagination for every paginated collection endpoint.

### Changed

- `mistershell_resource.external_id` is computed and nullable. Users no longer
  provide it; MisterShell extracts it during discovery when the native type has
  an external identity.
- Resource and credential type validation is performed against runtime catalogs
  instead of generated closed lists.
- The API client is split into small domain files with a shared HTTP core,
  bounded transport, retry handling, and consistent response envelopes.
- Acceptance tests require `TF_ACC=1`; live onboarding tests additionally use a
  reachable SSH target supplied through `MISTERSHELL_ACC_SSH_*` variables.

### Fixed

- MisterShell 2.0 request verbs and bodies for role/user assignments, ACLs,
  session-policy rules, auth providers, users, workers, and network resources.
- Singular data-source name lookups now re-fetch full detail responses where
  list endpoints return slim objects.

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
