resource "mistershell_metric_config" "health" {
  id = var.metric_key
  configuration = {
    is_enabled    = true
    display_order = 10
    options       = { warning_threshold = 80, critical_threshold = 95 }
  }
}
