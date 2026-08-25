# Read a single application setting by key.
data "mistershell_setting" "retention" {
  key = "security_log_retention_days"
}

# Values already use their native Terraform types.
output "retention_days" {
  value = data.mistershell_setting.retention.value
}

output "retention_default" {
  value = data.mistershell_setting.retention.default
}

output "retention_is_secret" {
  value = data.mistershell_setting.retention.is_secret
}
