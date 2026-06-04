# Look up an AI prompt by its integer ID.
data "mistershell_ai_prompt" "by_id" {
  id = 42
}

# Look up an AI prompt by its exact name.
# This works for both user-defined prompts and builtin "system" prompts:
# the builtin system prompts are read-only and only readable through the
# data source (they cannot be created or managed as a resource).
data "mistershell_ai_prompt" "general_assistant" {
  name = "builtin.general_assistant"
}

output "prompt_type" {
  value = data.mistershell_ai_prompt.general_assistant.type # => "system"
}

output "prompt_content" {
  value = data.mistershell_ai_prompt.general_assistant.content
}
