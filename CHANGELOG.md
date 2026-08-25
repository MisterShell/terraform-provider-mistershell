# Changelog

Notable changes to the MisterShell Terraform provider, for people writing HCL against it.

This project adheres to [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
