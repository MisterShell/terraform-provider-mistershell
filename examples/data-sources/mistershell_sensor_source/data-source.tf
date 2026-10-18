data "mistershell_sensor_source" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_sensor_source" "by_name" {
  name = "sensor-source"
}
