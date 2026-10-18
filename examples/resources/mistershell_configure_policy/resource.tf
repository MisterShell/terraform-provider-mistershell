resource "mistershell_configure_stack" "baseline" {
  name = "device-baseline"
}

resource "mistershell_configure_policy" "example" {
  name           = "device-baseline-policy"
  stack_id       = tonumber(mistershell_configure_stack.baseline.id)
  enabled        = false
  resource_types = ["cisco_ios"]
}
