
resource "mistershell_sensor_alert_rule" "example" {
  name          = "suppress-selected-alerts"
  enabled       = false
  suppress      = true
  signature_ids = [9900001]
}
