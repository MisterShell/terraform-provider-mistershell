data "mistershell_canary_service" "builtin" {
  id = var.builtin_canary_service_id
}

resource "mistershell_canary_service_override" "example" {
  service_id = tonumber(data.mistershell_canary_service.builtin.id)
  enabled    = false
}
