# Look up a credential by name
data "mistershell_credential" "by_name" {
  name = "dc-admin-ssh"
}

output "credential_type" {
  value = data.mistershell_credential.by_name.credential_type
}

# Look up a credential by ID
data "mistershell_credential" "by_id" {
  id = 1
}
