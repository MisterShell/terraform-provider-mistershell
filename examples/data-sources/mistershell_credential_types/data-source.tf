data "mistershell_credential_types" "installed" {}

output "credential_schemas" {
  value = { for definition in data.mistershell_credential_types.installed.types : definition.type => definition.schema }
}
