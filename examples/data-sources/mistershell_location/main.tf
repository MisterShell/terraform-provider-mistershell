# Look up a location by ID
data "mistershell_location" "existing" {
  id = 1
}

output "location_name" {
  value = data.mistershell_location.existing.name
}
