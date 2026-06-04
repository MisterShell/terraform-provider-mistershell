# Look up a tag by ID.
data "mistershell_tag" "by_id" {
  id = 1
}

# Look up a tag by name (exact match; must resolve to exactly one tag).
data "mistershell_tag" "by_name" {
  name = "production"
}

# Use the resolved attributes elsewhere.
output "production_resource_ids" {
  value = data.mistershell_tag.by_name.resource_ids
}

output "production_resource_count" {
  value = data.mistershell_tag.by_name.resource_count
}
