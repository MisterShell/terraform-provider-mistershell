# Anthropic model using an API key supplied from a sensitive variable.
# This model is marked as the default for agents that do not pin a model.
resource "mistershell_ai_model" "claude" {
  name           = "claude-sonnet"
  model_provider = "anthropic"
  model_id       = "claude-3-5-sonnet-latest"
  is_default     = true

  config = jsonencode({
    api_key = var.anthropic_api_key # secret; masked as "***" by the API, stored from config
  })
}

# Ollama model running locally — OpenAI-compatible API, no API key required.
# Only a base_url is needed (defaults to http://localhost:11434/v1 server-side).
resource "mistershell_ai_model" "ollama_llama" {
  name           = "local-llama"
  model_provider = "ollama"
  model_id       = "llama3.1"

  config = jsonencode({
    base_url = "http://localhost:11434/v1"
  })
}

# OpenAI model pointed at a custom gateway via base_url.
resource "mistershell_ai_model" "gpt" {
  name           = "gpt-4o"
  model_provider = "openai"
  model_id       = "gpt-4o"

  config = jsonencode({
    api_key  = var.openai_api_key # secret
    base_url = "https://gateway.example.com/v1"
  })
}

variable "anthropic_api_key" {
  type      = string
  sensitive = true
}

variable "openai_api_key" {
  type      = string
  sensitive = true
}
