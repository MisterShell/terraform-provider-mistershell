# Cisco IOS-XE switch
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "core-sw-01.zurich.example.com"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.1"
    port = 22
  })

  extra_data = jsonencode({
    rack     = "A-12"
    position = "U24"
  })
}

# AWS account
resource "mistershell_resource" "aws_account" {
  name          = "aws-production"
  resource_type = "aws_account"
  external_id   = "123456789012"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.aws_prod.id
}
