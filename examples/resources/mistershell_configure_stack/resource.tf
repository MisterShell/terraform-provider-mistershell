resource "mistershell_configure_template" "hostname" {
  name             = "device-hostname"
  body             = "hostname {{ vars.hostname }}"
  variables_schema = [{ name = "hostname", type = "string", required = true }]
}

resource "mistershell_configure_stack" "example" {
  name    = "device-baseline"
  members = [{ template_id = tonumber(mistershell_configure_template.hostname.id), position = 0 }]
}
