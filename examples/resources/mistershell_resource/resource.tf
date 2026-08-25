# Automatic onboarding: MisterShell connects through SSH, discovers the most
# specific installed native type, and falls back to Generic SSH when no native
# detector matches. A successful connection is required in either case.
resource "mistershell_resource" "automatic" {
  name          = "edge-router.zurich"
  engine_id     = "ssh"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = {
    host = "10.0.1.1"
    port = 22
  }
}

# Exact onboarding: when resource_type is supplied, MisterShell infers the
# engine and accepts the target only if that exact detector matches.
resource "mistershell_resource" "known_ios" {
  name          = "known-ios-router.zurich"
  resource_type = "cisco_ios"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = {
    host = "10.0.1.2"
    port = 22
  }
}

# Generic SSH is the exact-type special case: a successful SSH connection is
# sufficient and no native discovery is run. external_id is normally null.
resource "mistershell_resource" "generic_ssh" {
  name          = "shell-only-host.zurich"
  resource_type = "generic_ssh"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id

  connector_data = {
    host = "10.0.1.3"
    port = 22
  }
}

resource "mistershell_tag" "production" {
  name  = "production"
  color = "#e53935"
}

# tag_ids owns the complete tag set. Omit it to leave tags unmanaged; use []
# to actively keep the resource tag-free.
resource "mistershell_resource" "tagged" {
  name          = "tagged-router.zurich"
  engine_id     = "ssh"
  location_id   = mistershell_location.zurich.id
  credential_id = mistershell_credential.ssh_admin.id
  connector_data = {
    host = "10.0.1.4"
    port = 22
  }
  tag_ids = [mistershell_tag.production.id]
}
