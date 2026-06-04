# Read the entire permission registry.
data "mistershell_permissions" "all" {}

# Read only the permissions belonging to a single module.
data "mistershell_permissions" "tags" {
  module = "tags"
}

# Read permissions matching a fuzzy search term.
data "mistershell_permissions" "read_perms" {
  search = "read"
}

# The full list of module names (useful for the module filter above).
output "permission_modules" {
  value = data.mistershell_permissions.all.modules
}

# All permission names defined in the registry.
output "permission_names" {
  value = [for p in data.mistershell_permissions.all.permissions : p.name]
}

# Build a role from the tag-module permissions returned by the data source.
resource "mistershell_role" "tag_admin" {
  name        = "tag-admin"
  permissions = [for p in data.mistershell_permissions.tags.permissions : p.name]
}
