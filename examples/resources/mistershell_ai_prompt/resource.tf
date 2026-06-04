# A user-defined prompt whose content references a Jinja2 template variable.
# Variables are written as {{ variable }} and are substituted by the backend
# when an agent runs the prompt (see the resource docs for the full variable set).
resource "mistershell_ai_prompt" "resource_summary" {
  name = "concise-resource-summary"
  type = "user" # only "user" is allowed; "system" prompts are builtin/read-only

  content = <<-EOT
    Summarize resource {{ resource_id }} ("{{ resource_name }}", type {{ resource_type }}) concisely.
    Today is {{ current_date }}. Highlight only anomalies and the single most important follow-up.
  EOT

  description = "Short, anomaly-focused resource summary used by a custom agent."

  # Optional JSON Schema documenting the variables this prompt expects.
  # Stored from configuration; informational only (the backend does not
  # reject undeclared variables based on this schema).
  variable_schema = jsonencode({
    type = "object"
    properties = {
      resource_id   = { type = "integer", description = "Target resource ID." }
      resource_name = { type = "string", description = "Resource display name." }
      resource_type = { type = "string", description = "Resource type descriptor key." }
    }
    required = ["resource_id"]
  })
}
