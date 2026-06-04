# Look up by ID
data "mistershell_resource" "by_id" {
  id = 1
}

# Look up by name
data "mistershell_resource" "by_name" {
  name = "core-sw-01.zurich"
}

# Look up by name + type (narrows search)
data "mistershell_resource" "switch" {
  name          = "core-sw-01.zurich"
  resource_type = "cisco_iosxe"
}

# Access discovered metadata
output "switch_extra_data" {
  value = data.mistershell_resource.by_id.extra_data
}
