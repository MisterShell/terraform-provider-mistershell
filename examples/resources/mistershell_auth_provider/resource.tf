# An OIDC auth provider. config is a native per-provider_type Terraform object;
# client_secret is a secret (masked '****' by the API and
# stored from your configuration, not read back). mnemonic is immutable and is
# used in stable login and callback URLs.
#
# Create and update require an active license.
# OIDC and SAML also require the deployment's public URL before being enabled.
# Manage this instance-wide setting once, and share it across auth providers.
resource "mistershell_setting" "app_base_url" {
  key   = "app_base_url"
  value = jsonencode(var.mistershell_public_url)
}

resource "mistershell_auth_provider" "corp_oidc" {
  depends_on = [mistershell_setting.app_base_url]

  name          = "corp-oidc"
  mnemonic      = "corp-oidc"
  provider_type = "OIDC"
  is_enabled    = true
  display_order = 0

  config = {
    issuer_url    = "https://login.example.com"
    client_id     = "mistershell"
    client_secret = var.oidc_client_secret # secret
    scopes        = ["openid", "profile", "email"]
  }
}

variable "mistershell_public_url" {
  type        = string
  description = "Public URL users and identity providers use to reach MisterShell (e.g. https://mistershell.example.com)."
}

variable "oidc_client_secret" {
  type      = string
  sensitive = true
}
