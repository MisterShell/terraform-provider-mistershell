# Maps an external group (OIDC claim value here) to a MisterShell role.
# (provider_id, external_group) is unique. Create and update require an active
# license.
resource "mistershell_auth_provider_mapping" "admins" {
  provider_id    = mistershell_auth_provider.corp_oidc.id
  external_group = "platform-admins"
  role_id        = mistershell_role.admin.id
}
