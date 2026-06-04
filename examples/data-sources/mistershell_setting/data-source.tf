# Read a single application setting by key.
data "mistershell_setting" "retention" {
  key = "security_log_retention_days"
}

# The value is JSON; decode it to use the native type.
output "retention_days" {
  value = jsondecode(data.mistershell_setting.retention.value)
}

output "retention_default" {
  value = jsondecode(data.mistershell_setting.retention.default)
}

output "retention_is_secret" {
  value = data.mistershell_setting.retention.is_secret
}
