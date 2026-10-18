resource "mistershell_external_vault" "production" {
  name              = "Production Vault"
  mnemonic          = "production-vault"
  vault_type        = "hashicorp" # Also: openbao, aws_secrets_manager, azure_key_vault
  cache_ttl_seconds = 60

  config = {
    address           = "https://vault.example.com:8200"
    auth_method       = "approle"
    approle_role_id   = var.vault_role_id
    approle_secret_id = var.vault_secret_id
    kv_mount          = "secret"
    kv_version        = 2
  }
}

resource "mistershell_credential" "servers" {
  name              = "Server automation"
  credential_type   = "username_password"
  source            = "external"
  external_vault_id = mistershell_external_vault.production.id

  external_vault_ref = {
    base = { path = "servers/{{ resource.name }}" }
    fields = {
      username = "username"
      password = "password"
    }
  }
}

variable "vault_role_id" {
  type = string
}

variable "vault_secret_id" {
  type      = string
  sensitive = true
}
