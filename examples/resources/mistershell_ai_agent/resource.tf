# A complete, self-contained custom (user) agent: a model + a user prompt +
# a chat agent that references both. The agent is restricted to a single tool
# resolved by name; omit tool_ids entirely to allow ALL tools.

# 1. The model the agent will use (an Anthropic model marked default-eligible).
resource "mistershell_ai_model" "claude" {
  name           = "claude-sonnet"
  model_provider = "anthropic"
  model_id       = "claude-3-5-sonnet-latest"
  is_default     = true

  config = jsonencode({
    api_key = var.anthropic_api_key # secret; masked as "***" by the API, stored from config
  })
}

# 2. The system prompt the agent renders on every run (type "user").
resource "mistershell_ai_prompt" "assistant_prompt" {
  name = "ops-assistant-system"
  type = "user" # only "user" is creatable; "system" prompts are builtin/read-only

  content = <<-EOT
    You are an operations assistant for MisterShell.
    Today is {{ current_date }} at {{ current_time }}.
    Be concise. Prefer tool calls over speculation, and surface anomalies first.
  EOT

  description = "System prompt for a custom operations chat agent."
}

# 3. Resolve a backend tool by name so we can restrict the agent to it.
#    Tools are backend-builtin/read-only; look them up, never create them.
data "mistershell_ai_tool" "list_resources" {
  name = "list_resources"
}

# 4. The custom chat agent, wiring the model + prompt + tool restriction.
resource "mistershell_ai_agent" "ops_assistant" {
  name        = "ops-assistant"
  type        = "chat" # "chat" or "background"; immutable (changing it replaces the agent)
  description = "Operations chat assistant restricted to read-only resource listing."

  model_id         = mistershell_ai_model.claude.id            # omit to use the default model
  system_prompt_id = mistershell_ai_prompt.assistant_prompt.id # required

  # Agent-side execution config. Stored from configuration (the server may
  # enrich/reorder this blob) and excluded from import — re-supply after import.
  config = jsonencode({
    token_budget = 120000 # max total tokens of context the run may consume
    temperature  = 0.2    # sampling temperature override (agent-side, not model-side)
  })

  # Restrict the agent to these tool IDs. Omit tool_ids entirely (or set it to
  # the empty set) to allow ALL tools.
  tool_ids = [data.mistershell_ai_tool.list_resources.id]
}

variable "anthropic_api_key" {
  type      = string
  sensitive = true
}
