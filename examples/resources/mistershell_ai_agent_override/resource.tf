data "mistershell_ai_agent" "builtin" {
  name = "General Assistant"
}

resource "mistershell_ai_agent_override" "builtin" {
  agent_id = data.mistershell_ai_agent.builtin.id
  config = {
    temperature = 0.2
    max_steps   = 12
  }
}
