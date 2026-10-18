data "mistershell_ca_certificate" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_ca_certificate" "by_name" {
  name = "corporate-ca"
}
