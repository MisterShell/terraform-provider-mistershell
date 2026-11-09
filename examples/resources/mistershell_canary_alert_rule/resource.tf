resource "mistershell_canary_alert_rule" "example" {
  name          = "site-canary-alerts"
  enabled       = true
  location_ids  = [var.location_id]
  service_types = ["ssh"]
  log           = true
  notify        = true
}
