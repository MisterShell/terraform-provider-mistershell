# AI tools are backend-builtin and read-only. Look one up by its exact name to
# resolve the integer ID you need for an agent's tool_ids.
data "mistershell_ai_tool" "search" {
  name = "search"
}

data "mistershell_ai_tool" "inspect" {
  name = "inspect"
}

# You can also look up a tool directly by ID.
data "mistershell_ai_tool" "by_id" {
  id = 1
}

output "search_tool_permission" {
  value = data.mistershell_ai_tool.search.required_permission
}

# Feed the resolved tool IDs into an agent's tool_ids.
resource "mistershell_ai_agent" "assistant" {
  name = "inventory-assistant"
  type = "chat"

  tool_ids = [
    data.mistershell_ai_tool.search.id,
    data.mistershell_ai_tool.inspect.id,
  ]
}
