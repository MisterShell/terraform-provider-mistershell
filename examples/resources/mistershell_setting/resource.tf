# A setting value uses its native Terraform type (bool / number / string).
# The key set is fixed by the backend registry.

# Boolean setting.
resource "mistershell_setting" "session_resume" {
  key   = "enable_session_resume"
  value = true
}

# Integer setting.
resource "mistershell_setting" "security_log_retention" {
  key   = "security_log_retention_days"
  value = 120
}

# String setting.
resource "mistershell_setting" "smtp_from" {
  key   = "smtp_from_email"
  value = "alerts@example.com"
}
