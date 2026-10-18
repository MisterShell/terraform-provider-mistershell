
resource "mistershell_sensor_custom_rule" "example" {
  name    = "icmp-audit"
  kind    = "local"
  enabled = false
  content = "alert icmp any any -> any any (msg:\"ICMP audit\"; sid:9900001; rev:1;)"
}
