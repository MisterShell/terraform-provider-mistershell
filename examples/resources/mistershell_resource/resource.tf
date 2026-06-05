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

# Tagging a resource from the resource side.
#
# When `tag_ids` is set, the provider manages the resource's tags EXCLUSIVELY:
# on every apply it adds/removes tags so the live set equals `tag_ids` exactly.
# Reference each tag by id (this also creates the proper dependency ordering).
#
# Own each tag<->resource edge from ONE side only: set `tag_ids` here OR list the
# resource in `mistershell_tag.resource_ids`, never both.
#   - Omit `tag_ids` entirely (null) to leave tags UNMANAGED (manage them from
#     the tag side instead); the live tags are still read back into `tags`.
#   - Set `tag_ids = []` to keep the resource exclusively tag-free.
resource "mistershell_tag" "prod" {
  name  = "production"
  color = "#e53935"
}

resource "mistershell_tag" "zurich" {
  name  = "zurich"
  color = "blue"
}

resource "mistershell_resource" "tagged_switch" {
  name          = "edge-sw-01.zurich"
  resource_type = "cisco_iosxe"
  external_id   = "FCW2345L0CD"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = jsonencode({
    host = "10.0.1.2"
  })

  # Exclusively manage this resource's tags from the resource side.
  tag_ids = [
    mistershell_tag.prod.id,
    mistershell_tag.zurich.id,
  ]
}

# `tags` is computed (read-back) and always reflects the live tags on the
# resource, whether or not `tag_ids` is managed.
output "tagged_switch_tags" {
  value = mistershell_resource.tagged_switch.tags
}
