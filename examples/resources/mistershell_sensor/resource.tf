
resource "mistershell_sensor" "example" {
  name        = "site-sensor"
  description = "Sensor registration; deploy the returned token separately."
  location_id = var.location_id
  is_enabled  = false
  config      = { interfaces = ["eth0"], home_net = ["192.0.2.0/24"] }
}
