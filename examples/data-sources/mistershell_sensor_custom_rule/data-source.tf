data "mistershell_sensor_custom_rule" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_sensor_custom_rule" "by_name" {
  name = "sensor-custom-rule"
}
