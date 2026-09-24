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

# An approval gate: members of the "on-call-leads" role must approve new
# connections to production databases. Approve rules are evaluated before all
# accept/deny rules regardless of position, and cannot reference command ACLs.
resource "mistershell_session_policy_rule" "approve_prod_db" {
  name           = "approve-production-databases"
  action         = "approve"
  resource_types = ["database_postgres", "database_mysql"]
  location_ids   = [mistershell_location.production.id]

  approval = {
    approver_role_ids  = [mistershell_role.on_call_leads.id]
    expiration_seconds = 3600 # pending requests expire after 1 hour
    validity_seconds   = 1800 # an approval grants 30 minutes of access
    notify_email       = true
  }
}
