# Look up a role by ID.
data "mistershell_role" "by_id" {
  id = 1
}

# Look up a role by name (exact match; must resolve to exactly one role).
data "mistershell_role" "by_name" {
  name = "auditor"
}

# Use the resolved attributes elsewhere.
output "auditor_permissions" {
  value = data.mistershell_role.by_name.permissions
}

output "auditor_scope_location_ids" {
  value = data.mistershell_role.by_name.scope_location_ids
}
