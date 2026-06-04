# A read-only auditor role scoped to two locations.
# scope_location_ids limits the role to the given location subtrees;
# omit it (or leave it empty) to grant the role across all locations.
resource "mistershell_role" "auditor" {
  name        = "auditor"
  description = "Read-only access to tags and resources in EMEA"

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
  name        = "superuser"
  description = "Full administrative access"

  permissions = ["*.*.*"]
}
