
resource "mistershell_proxy" "example" {
  name        = "site-proxy"
  description = "Proxy registration; deploy the returned token separately."
  location_id = var.location_id
  is_enabled  = false
}
