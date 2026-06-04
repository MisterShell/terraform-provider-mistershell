# A setting value is JSON; use jsonencode() to match the key's native type
# (bool / int / string). The key set is fixed by the backend registry.

# Boolean setting.
resource "mistershell_setting" "session_resume" {
  key   = "enable_session_resume"
  value = jsonencode(true)
}

# Integer setting.
resource "mistershell_setting" "security_log_retention" {
  key   = "security_log_retention_days"
  value = jsonencode(120)
}

# String setting.
resource "mistershell_setting" "smtp_from" {
  key   = "smtp_from_email"
  value = jsonencode("alerts@example.com")
}
