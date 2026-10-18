data "mistershell_proxy" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_proxy" "by_name" {
  name = "proxy"
}
