data "mistershell_ai_skill" "builtin" {
  name = "arista-eos-platform-brief"
}

resource "mistershell_ai_skill_override" "builtin" {
  skill_id   = data.mistershell_ai_skill.builtin.id
  is_enabled = false
}
