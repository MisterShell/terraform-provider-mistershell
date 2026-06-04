# Look up by ID
data "mistershell_location" "by_id" {
  id = 1
}

# Look up by name
data "mistershell_location" "by_name" {
  name = "Zurich DC"
}

# Look up by name within a specific parent.
# parent_id can be a literal ID or a reference to another location.
data "mistershell_location" "child" {
  name      = "Zurich DC"
  parent_id = 1 # ID of the parent location (e.g. "Switzerland")
}
