# An OIDC auth provider. config is an opaque per-provider_type JSON blob set
# with jsonencode(); client_secret is a secret (masked '****' by the API and
# stored from your configuration, not read back). display_order is set
# declaratively via a post-create PATCH; assign distinct values for a
# deterministic login-page order.
#
# Create and update require an active license.
resource "mistershell_auth_provider" "corp_oidc" {
  name          = "corp-oidc"
  provider_type = "OIDC"
  is_enabled    = true
  display_order = 0

  config = jsonencode({
    issuer_url    = "https://login.example.com"
    client_id     = "mistershell"
    client_secret = var.oidc_client_secret # secret
    scopes        = ["openid", "profile", "email"]
  })
}

variable "oidc_client_secret" {
  type      = string
  sensitive = true
}
