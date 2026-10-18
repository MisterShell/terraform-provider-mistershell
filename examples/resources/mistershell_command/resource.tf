
resource "mistershell_command" "example" {
  name          = "show-platform-version"
  resource_type = "cisco_ios"
  description   = "Read device version."
  category      = "Inventory"
  mode          = "read"
  definition    = { command = "show version" }
  parse_source  = { kind = "no_parse" }
}
