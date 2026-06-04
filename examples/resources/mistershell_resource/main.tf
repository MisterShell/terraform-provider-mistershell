# Cisco IOS-XE switch
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "FCW2345L0AB"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.1"
    port = 22
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

# Windows server (SSH/OpenSSH + RDP)
resource "mistershell_resource" "win_jumpbox" {
  name          = "win-jump-01.zurich"
  resource_type = "windows"
  external_id   = "WIN-JUMP-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.rdp_admin.id

  connector_data = jsonencode({
    host            = "10.0.2.10"
    port            = 22
    rdp_port        = 3389
    nla_required    = true
    keyboard_layout = "0x0000040C"
  })
}

# SQL database (via usql)
resource "mistershell_resource" "app_db" {
  name          = "app-postgres-01.zurich"
  resource_type = "database"
  external_id   = "app-postgres-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.app_db.id

  connector_data = jsonencode({
    engine = "postgres"
    host   = "10.0.3.20"
    port   = 5432
  })
}
