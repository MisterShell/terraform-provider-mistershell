# Look up an AI model by its integer ID.
data "mistershell_ai_model" "by_id" {
  id = 123
}

# Look up an AI model by its exact name.
data "mistershell_ai_model" "by_name" {
  name = "claude-sonnet"
}

output "model_provider" {
  value = data.mistershell_ai_model.by_name.model_provider
}

output "model_id" {
  value = data.mistershell_ai_model.by_name.model_id
}

output "model_is_default" {
  value = data.mistershell_ai_model.by_name.is_default
}
