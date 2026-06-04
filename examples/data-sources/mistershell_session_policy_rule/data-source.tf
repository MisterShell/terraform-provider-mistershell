# Look up a session-policy rule by its integer ID.
data "mistershell_session_policy_rule" "by_id" {
  id = 7
}

# Look up a session-policy rule by its exact name. Rule names are not unique
# server-side, so this errors if more than one rule matches.
data "mistershell_session_policy_rule" "by_name" {
  name = "deny-dangerous-shell"
}

output "rule_position" {
  value = data.mistershell_session_policy_rule.by_name.position
}

output "rule_action" {
  value = data.mistershell_session_policy_rule.by_name.action
}
