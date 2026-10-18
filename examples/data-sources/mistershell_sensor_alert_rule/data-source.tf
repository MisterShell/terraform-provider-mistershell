data "mistershell_sensor_alert_rule" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_sensor_alert_rule" "by_name" {
  name = "sensor-alert-rule"
}
