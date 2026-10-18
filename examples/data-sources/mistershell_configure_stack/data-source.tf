data "mistershell_configure_stack" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_configure_stack" "by_name" {
  name = "configure-stack"
}
