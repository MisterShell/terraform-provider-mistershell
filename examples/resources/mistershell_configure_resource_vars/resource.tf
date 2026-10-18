resource "mistershell_configure_template" "hostname" {
  name             = "device-hostname"
  body             = "hostname {{ vars.hostname }}"
  variables_schema = [{ name = "hostname", type = "string", required = false }]
}
resource "mistershell_configure_stack" "baseline" {
  name    = "device-baseline"
  members = [{ template_id = tonumber(mistershell_configure_template.hostname.id), position = 0 }]
}

resource "mistershell_configure_resource_vars" "example" {
  resource_id = var.resource_id
  stack_id    = tonumber(mistershell_configure_stack.baseline.id)
  values      = { hostname = "edge-router-01" }
}
