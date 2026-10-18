data "mistershell_command" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_command" "by_name" {
  name = "command"
}
