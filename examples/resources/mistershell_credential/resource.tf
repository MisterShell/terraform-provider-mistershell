# SSH password credential (for SSH-family resources and windows)
resource "mistershell_credential" "ssh_admin" {
  name            = "dc-admin-ssh"
  credential_type = "ssh_password"
  description     = "Data center admin SSH credentials"

  credential_data = jsonencode({
    username        = "admin"
    password        = var.ssh_password
    enable_password = var.enable_password # optional
  })
}

# SSH key credential (for SSH-family resources and windows)
resource "mistershell_credential" "ssh_key" {
  name            = "dc-admin-ssh-key"
  credential_type = "ssh_key"

  credential_data = jsonencode({
    username    = "admin"
    private_key = var.ssh_private_key # PEM, multiline
    passphrase  = ""                  # optional
  })
}

# AWS credential (for aws_account resources)
resource "mistershell_credential" "aws_prod" {
  name            = "aws-production"
  credential_type = "aws_credentials"

  credential_data = jsonencode({
    access_key = var.aws_access_key
    secret_key = var.aws_secret_key
  })
}

# Azure service principal (for azure_subscription resources)
resource "mistershell_credential" "azure_prod" {
  name            = "azure-production"
  credential_type = "azure_service_principal"

  credential_data = jsonencode({
    client_id     = var.azure_client_id
    client_secret = var.azure_client_secret
  })
}

# Kubeconfig credential (for kubernetes_cluster resources)
resource "mistershell_credential" "k8s" {
  name            = "prod-cluster-kubeconfig"
  credential_type = "kubeconfig"

  credential_data = jsonencode({
    kubeconfig = var.kubeconfig # full kubeconfig YAML
  })
}

# RDP credential (for windows / generic_rdp resources)
resource "mistershell_credential" "rdp_admin" {
  name            = "win-rdp-admin"
  credential_type = "rdp_password"

  credential_data = jsonencode({
    username = "Administrator"
    domain   = "CORP" # optional
    password = var.rdp_password
  })
}

# Database credential (for database resources)
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

variable "ssh_private_key" {
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

variable "azure_client_id" {
  type = string
}

variable "azure_client_secret" {
  type      = string
  sensitive = true
}

variable "kubeconfig" {
  type      = string
  sensitive = true
}
