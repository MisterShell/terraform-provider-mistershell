data "mistershell_external_vault_types" "installed" {}

output "vault_reference_schemas" {
  value = { for definition in data.mistershell_external_vault_types.installed.types : definition.vault_type => definition.base_ref_schema }
}
