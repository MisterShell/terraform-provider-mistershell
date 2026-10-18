
resource "mistershell_recording_rule" "example" {
  name    = "excluded-recordings"
  action  = "skip"
  enabled = false
  comment = "Configure scope before enabling this rule."
}
