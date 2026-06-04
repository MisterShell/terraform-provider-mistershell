# Look up an AI skill by its integer ID.
data "mistershell_ai_skill" "by_id" {
  id = 42
}

# Look up an AI skill by its exact name.
# This works for both custom skills and builtin (MisterShell-managed) skills:
# builtin skills (is_builtin = true) are read-only and only readable through the
# data source — they cannot be created or managed as a resource.
data "mistershell_ai_skill" "linux_brief" {
  name = "linux-platform-brief"
}

output "skill_is_builtin" {
  value = data.mistershell_ai_skill.linux_brief.is_builtin # => true
}

output "skill_resource_types" {
  value = data.mistershell_ai_skill.linux_brief.resource_types # => ["linux"]
}
