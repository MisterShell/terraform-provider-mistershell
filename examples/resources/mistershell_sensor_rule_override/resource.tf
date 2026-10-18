resource "mistershell_sensor_rule_override" "rule" {
  gid    = 1
  sid    = 2024218
  action = "disable"
}

# Review and apply staged ruleset changes separately in MisterShell.
