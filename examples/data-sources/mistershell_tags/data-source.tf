# List every tag.
data "mistershell_tags" "all" {}

# List only tags whose name contains "prod" (case-insensitive substring search).
data "mistershell_tags" "prod" {
  search = "prod"
}

# All tag names.
output "all_tag_names" {
  value = [for t in data.mistershell_tags.all.tags : t.name]
}

# Each element exposes { id, name, color, description, resource_count }.
output "prod_tags" {
  value = data.mistershell_tags.prod.tags
}

# Map of tag name -> number of resources assigned that tag.
output "tag_resource_counts" {
  value = { for t in data.mistershell_tags.all.tags : t.name => t.resource_count }
}
