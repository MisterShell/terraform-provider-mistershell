# Look up a session-policy ACL by its integer ID.
data "mistershell_session_policy_acl" "by_id" {
  id = 42
}

# Look up a session-policy ACL by its exact name.
data "mistershell_session_policy_acl" "by_name" {
  name = "dangerous-commands"
}

output "acl_patterns" {
  value = data.mistershell_session_policy_acl.by_name.patterns
}
