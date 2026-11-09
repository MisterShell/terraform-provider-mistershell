resource "mistershell_syslog_rule" "example" {
  name         = "site-syslog-errors"
  enabled      = true
  severities   = [0, 1, 2, 3]
  location_ids = [var.location_id]
  log          = true
  forward      = true
}
