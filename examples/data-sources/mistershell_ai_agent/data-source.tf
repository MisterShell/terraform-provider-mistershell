# Look up an AI agent by its integer ID.
data "mistershell_ai_agent" "by_id" {
  id = 123
}

# Look up an AI agent by its exact name. This also works for builtin agents,
# which are managed by MisterShell and exposed read-only here
# (e.g. "General Assistant", "Resource Summary", "Session Assist").
data "mistershell_ai_agent" "by_name" {
  name = "General Assistant"
}

output "agent_type" {
  value = data.mistershell_ai_agent.by_name.type
}

output "agent_is_builtin" {
  value = data.mistershell_ai_agent.by_name.is_builtin
}

output "agent_is_functional" {
  value = data.mistershell_ai_agent.by_name.is_functional
}

output "agent_tool_ids" {
  value = data.mistershell_ai_agent.by_name.tool_ids
}
