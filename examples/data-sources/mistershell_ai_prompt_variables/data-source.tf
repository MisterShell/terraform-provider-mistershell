data "mistershell_ai_prompt_variables" "catalog" {}

output "universal_prompt_variables" {
  value = data.mistershell_ai_prompt_variables.catalog.universal
}
