data "mistershell_sensor_rule_override" "rule" {
  gid = 1
  sid = 2024218
}

output "rule_change_pending" {
  value = data.mistershell_sensor_rule_override.rule.pending
}
