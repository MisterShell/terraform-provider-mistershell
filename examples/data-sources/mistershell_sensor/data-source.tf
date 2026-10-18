data "mistershell_sensor" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_sensor" "by_name" {
  name = "sensor"
}
