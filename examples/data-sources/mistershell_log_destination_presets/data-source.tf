# Read the code-owned log-destination presets.
data "mistershell_log_destination_presets" "all" {}

# All available preset keys.
output "preset_keys" {
  value = [for p in data.mistershell_log_destination_presets.all.presets : p.key]
}

# Build a destination from the Splunk HEC preset's default_config, overriding
# the URL and the (secret) header value.
locals {
  splunk = one([
    for p in data.mistershell_log_destination_presets.all.presets : p
    if p.key == "splunk_hec"
  ])
}

resource "mistershell_log_destination" "splunk" {
  name    = "splunk-hec"
  type    = local.splunk.type
  streams = ["security"]

  config = jsonencode(merge(jsondecode(local.splunk.default_config), {
    url = "https://splunk.example.com:8088/services/collector"
    auth = {
      type         = "header"
      header_name  = "Authorization"
      header_value = "Splunk ${var.splunk_token}"
    }
  }))
}

variable "splunk_token" {
  type      = string
  sensitive = true
}
