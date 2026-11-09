resource "mistershell_metric_customizations" "thresholds" {
  id = mistershell_metric_config.health.id
  configuration = {
    items = [{
      tag_id          = mistershell_tag.production.id
      skip_collection = false
      thresholds      = { warning_threshold = 80, critical_threshold = 95 }
    }]
  }
}
