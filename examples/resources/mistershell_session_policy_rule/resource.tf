# A deny rule placed early in the chain. Lower position runs first; assign
# distinct values (e.g. 10, 20, 30) for deterministic ordering.
resource "mistershell_session_policy_rule" "deny_dangerous" {
  name     = "deny-dangerous-shell"
  position = 10
  action   = "deny"

  session_types   = ["shell"]
  resource_types  = ["linux", "generic_ssh"]
  command_acl_ids = [mistershell_session_policy_acl.dangerous_commands.id]

  notify = true
  log    = true
}

# A broad accept rule placed later (lower priority). Empty selector sets
# (resource_types, session_types, etc.) mean "Any".
resource "mistershell_session_policy_rule" "allow_rest" {
  name     = "allow-everything-else"
  position = 20
  action   = "accept"
}
