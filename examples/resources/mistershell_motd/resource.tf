
resource "mistershell_motd" "example" {
  title      = "Maintenance notice"
  content    = "Scheduled maintenance is planned for Sunday."
  icon       = "info"
  is_enabled = false
}
