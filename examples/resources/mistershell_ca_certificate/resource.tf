
resource "mistershell_ca_certificate" "example" {
  name    = "corporate-ca"
  pem     = file("${path.module}/corporate-ca.pem")
  enabled = true
}
