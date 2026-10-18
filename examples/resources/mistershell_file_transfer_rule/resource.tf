
resource "mistershell_file_transfer_rule" "example" {
  name    = "deny-transfers"
  action  = "deny"
  enabled = false
  comment = "Configure scope before enabling this rule."
}
