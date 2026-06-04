# Look up an auth provider by its integer ID.
data "mistershell_auth_provider" "by_id" {
  id = 3
}

# Look up an auth provider by its exact name (names are unique).
data "mistershell_auth_provider" "by_name" {
  name = "corp-oidc"
}

output "provider_type" {
  value = data.mistershell_auth_provider.by_name.provider_type
}

# Note: the config output reflects the server's stored config with secret
# fields masked as '****' — secrets cannot be recovered from this data source.
output "mappings_count" {
  value = data.mistershell_auth_provider.by_name.group_mappings_count
}
