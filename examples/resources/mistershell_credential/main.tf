# SSH password credential
resource "mistershell_credential" "ssh_admin" {
  name            = "dc-admin-ssh"
  credential_type = "ssh_password"
  description     = "Data center admin SSH credentials"

  credential_data = jsonencode({
    username        = "admin"
    password        = var.ssh_password
    enable_password = var.enable_password
  })
}

# AWS credential
resource "mistershell_credential" "aws_prod" {
  name            = "aws-production"
  credential_type = "aws_credentials"

  credential_data = jsonencode({
    access_key = var.aws_access_key
    secret_key = var.aws_secret_key
    region     = "eu-west-1"
  })
}

# RDP credential (for windows / generic_rdp resources)
resource "mistershell_credential" "rdp_admin" {
  name            = "win-rdp-admin"
  credential_type = "rdp_password"

  credential_data = jsonencode({
    username = "Administrator"
    domain   = "CORP"
    password = var.rdp_password
  })
}

# Database credential (for database resources, via usql)
resource "mistershell_credential" "app_db" {
  name            = "app-db-user"
  credential_type = "db_password"

  credential_data = jsonencode({
    username = "app"
    password = var.db_password
  })
}

variable "ssh_password" {
  type      = string
  sensitive = true
}

variable "rdp_password" {
  type      = string
  sensitive = true
}

variable "db_password" {
  type      = string
  sensitive = true
}

variable "enable_password" {
  type      = string
  sensitive = true
  default   = ""
}

variable "aws_access_key" {
  type      = string
  sensitive = true
}

variable "aws_secret_key" {
  type      = string
  sensitive = true
}
