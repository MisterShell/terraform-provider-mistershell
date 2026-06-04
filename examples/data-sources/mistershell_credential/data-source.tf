# Look up by ID
data "mistershell_credential" "by_id" {
  id = 1
}

# Look up by unique name
data "mistershell_credential" "by_name" {
  name = "dc-admin-ssh"
}

# Look up by type (must match exactly one)
data "mistershell_credential" "k8s" {
  credential_type = "kubeconfig"
}
