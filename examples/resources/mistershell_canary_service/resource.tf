resource "mistershell_canary_service" "example" {
  name         = "site-ssh-decoy"
  service_type = "ssh"
  enabled      = false
  location_ids = [var.location_id]
  settings = {
    port   = 2222
    banner = "SSH-2.0-OpenSSH_9.6"
  }
}
