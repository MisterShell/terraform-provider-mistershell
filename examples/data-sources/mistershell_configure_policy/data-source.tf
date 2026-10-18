data "mistershell_configure_policy" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_configure_policy" "by_name" {
  name = "configure-policy"
}
