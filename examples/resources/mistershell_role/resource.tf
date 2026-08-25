# A read-only auditor role scoped to two locations.
# restricted mode requires one or more location scope roots.
resource "mistershell_role" "auditor" {
  name                = "auditor"
  description         = "Read-only access to tags and resources in EMEA"
  location_scope_mode = "restricted"

  scope_location_ids = [
    mistershell_location.emea.id,
    mistershell_location.apac.id,
  ]

  permissions = [
    "app.tags.read",
    "app.resources.read",
  ]
}

# A global superuser role. The '*.*.*' permission cannot be combined with any
# other permission, and cannot be used together with scope_location_ids.
resource "mistershell_role" "superuser" {
  name                = "superuser"
  description         = "Full administrative access"
  location_scope_mode = "global"

  permissions = ["*.*.*"]
}
