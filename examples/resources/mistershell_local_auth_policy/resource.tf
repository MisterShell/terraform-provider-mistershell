resource "mistershell_local_auth_policy" "local" {
  id            = "local"
  configuration = { mfa_method = "totp" }
}
