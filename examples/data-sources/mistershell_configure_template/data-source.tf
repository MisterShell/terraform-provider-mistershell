data "mistershell_configure_template" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_configure_template" "by_name" {
  name = "configure-template"
}
