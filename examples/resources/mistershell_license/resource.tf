variable "license_key" {
  type      = string
  sensitive = true
}

resource "mistershell_license" "example" {
  license_key = var.license_key
  # Enable only after reviewing the capacity impact.
  confirm_over_capacity         = false
  confirm_removal_over_capacity = false

  # Renewals use a distinct license ID: install first, then withdraw the old grant.
  lifecycle {
    create_before_destroy = true
  }
}
