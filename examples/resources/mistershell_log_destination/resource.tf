# Syslog destination over TLS, forwarding security and policy logs.
resource "mistershell_log_destination" "syslog" {
  name         = "central-syslog"
  type         = "syslog"
  enabled      = true
  streams      = ["security", "policy"]
  min_severity = "medium"

  config = jsonencode({
    type       = "syslog"
    host       = "logs.example.com"
    port       = 6514
    protocol   = "TLS"
    format     = "RFC5424"
    facility   = "local1"
    tls_verify = true
  })
}

# Webhook destination with bearer-token auth, forwarding all streams.
resource "mistershell_log_destination" "webhook" {
  name         = "siem-webhook"
  type         = "webhook"
  enabled      = true
  streams      = ["security", "policy", "api", "app"]
  min_severity = "info"

  config = jsonencode({
    type            = "webhook"
    url             = "https://siem.example.com/ingest"
    method          = "POST"
    body_format     = "raw"
    timeout_seconds = 5
    tls_verify      = true
    headers = {
      "X-Source" = "mistershell"
    }
    auth = {
      type  = "bearer"
      token = var.webhook_token # secret; masked by the API, stored from config
    }
  })
}

variable "webhook_token" {
  type      = string
  sensitive = true
}
