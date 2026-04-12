# Look up by ID
data "mistershell_location" "by_id" {
  id = 1
}

# Look up by name
data "mistershell_location" "by_name" {
  name = "Zurich DC"
}

# Look up by name within a specific parent
data "mistershell_location" "child" {
  name      = "Zurich DC"
  parent_id = mistershell_location.europe.id
}
