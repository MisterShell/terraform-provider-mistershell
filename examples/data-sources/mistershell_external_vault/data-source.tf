data "mistershell_external_vault" "production" {
  mnemonic = "production-vault"
  # Alternatively use id = 42 or name = "Production Vault".
}

output "vault_type" {
  value = data.mistershell_external_vault.production.vault_type
}
