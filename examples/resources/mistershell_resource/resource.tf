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

# For an existing resource, keep its Terraform address and change these two
# attributes to park it: location_id and is_enabled. Its ID is preserved.
# A newly created resource still requires successful live onboarding.
resource "mistershell_resource" "parked" {
  name          = "parked-router"
  engine_id     = "ssh"
  location_id   = mistershell_location.parking.id
  credential_id = mistershell_credential.ssh_admin.id
  is_enabled    = false # Pause automatic snapshots; manual operations remain available.

  connector_data = {
    host = "10.0.1.5"
    port = 22
  }
}

# Native SSH automation: accept a user's registered personal SSH key (My
# Account > Account Security > My SSH keys) on its own, in place of the account
# password and second authentication factor (MFA), for direct native SSH
# sessions to this resource through the MisterShell SSH gateway. The resource
# credential, session policy, recording, and audit still apply. Only resource
# types with a shell session mode accept it; MisterShell rejects it on
# generic_rdp, generic_vnc, and generic_web with HTTP 400. Requires
# MisterShell >= 2.0.15.
resource "mistershell_resource" "automation_target" {
  name                     = "ci-runner-host.zurich"
  resource_type            = "generic_ssh"
  location_id              = mistershell_location.zurich.id
  credential_id            = mistershell_credential.ssh_admin.id
  sshgw_allow_personal_key = true

  connector_data = {
    host = "10.0.1.6"
    port = 22
  }
}
