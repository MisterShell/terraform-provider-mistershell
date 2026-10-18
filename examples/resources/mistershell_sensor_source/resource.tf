
resource "mistershell_sensor_source" "example" {
  name    = "corporate-rules"
  url     = "https://rules.example.com/corporate.rules"
  enabled = false
}
