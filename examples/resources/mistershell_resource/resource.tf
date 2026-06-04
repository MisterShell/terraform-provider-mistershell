# SSH device (Cisco IOS-XE switch) -> ssh_password / ssh_key
resource "mistershell_resource" "core_switch" {
  name          = "core-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "FCW2345L0AB"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.1"
    port = 22 # optional, default 22
  })
}

# Windows server (SSH/OpenSSH + RDP) -> ssh_password (also accepts ssh_key, rdp_password)
resource "mistershell_resource" "win_jumpbox" {
  name          = "win-jump-01.zurich"
  resource_type = "windows"
  external_id   = "WIN-JUMP-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host            = "10.0.2.10"
    port            = 22           # SSH/OpenSSH port, default 22
    rdp_port        = 3389         # default 3389
    nla_required    = true         # default true
    keyboard_layout = "0x0000040C" # "" = en-US
  })
}

# RDP-only host -> rdp_password
resource "mistershell_resource" "rdp_host" {
  name          = "rdp-app-01.zurich"
  resource_type = "generic_rdp"
  external_id   = "RDP-APP-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.rdp_admin.id

  connector_data = jsonencode({
    host         = "10.0.2.20"
    rdp_port     = 3389 # default 3389
    nla_required = true # default true
  })
}

# SQL database -> db_password
resource "mistershell_resource" "app_db" {
  name          = "app-postgres-01.zurich"
  resource_type = "database"
  external_id   = "app-postgres-01"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.app_db.id

  connector_data = jsonencode({
    engine = "postgres" # postgres | mysql | sqlserver | clickhouse
    host   = "10.0.3.20"
    port   = 5432 # optional, engine default (postgres 5432)
  })
}

# AWS account -> aws_credentials
resource "mistershell_resource" "aws_account" {
  name          = "aws-production"
  resource_type = "aws_account"
  external_id   = "123456789012"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.aws_prod.id

  connector_data = jsonencode({
    hostname = "prod-account" # optional account alias
  })
}

# Azure subscription -> azure_service_principal
resource "mistershell_resource" "azure_subscription" {
  name          = "azure-production"
  resource_type = "azure_subscription"
  external_id   = "00000000-0000-0000-0000-000000000003"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.azure_prod.id

  connector_data = jsonencode({
    tenant_id       = "00000000-0000-0000-0000-000000000002"
    subscription_id = "00000000-0000-0000-0000-000000000003"
    cloud           = "AzureCloud" # default AzureCloud
  })
}

# Kubernetes cluster -> kubeconfig
resource "mistershell_resource" "k8s_cluster" {
  name          = "prod-cluster"
  resource_type = "kubernetes_cluster"
  external_id   = "prod-cluster"
  location_id   = mistershell_location.emea.id
  credential_id = mistershell_credential.k8s.id

  connector_data = jsonencode({
    context   = "prod" # optional kubeconfig context
    namespace = "default"
  })
}
