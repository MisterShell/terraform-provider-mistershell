resource "mistershell_canary" "example" {
  name               = "site-canary"
  location_id        = var.location_id
  is_enabled         = false
  description        = "Canary registration; deploy its enrollment token separately."
  decoy_interface    = "eth1"
  ip_ignorelist      = ["192.0.2.0/24"]
  rate_limit_per_min = 120
}
