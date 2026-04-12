# Look up a network resource by ID
data "mistershell_resource" "switch" {
  id = 1
}

output "switch_status" {
  value = data.mistershell_resource.switch.status
}
