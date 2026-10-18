data "mistershell_auth_provider_mapping" "engineering" {
  provider_id    = var.auth_provider_id
  external_group = "engineering"
}

# Direct lookup within the same authentication provider.
data "mistershell_auth_provider_mapping" "by_id" {
  provider_id = var.auth_provider_id
  id          = 123
}
