
resource "mistershell_configure_template" "example" {
  name             = "device-hostname"
  body             = "hostname {{ vars.hostname }}"
  variables_schema = [{ name = "hostname", type = "string", required = true }]
}
