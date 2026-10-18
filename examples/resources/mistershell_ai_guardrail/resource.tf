resource "mistershell_ai_guardrail" "ssh" {
  resource_type     = "generic_ssh"
  allowlist_enabled = true
}
