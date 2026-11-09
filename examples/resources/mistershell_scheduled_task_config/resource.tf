data "mistershell_scheduled_task_config" "cleanup" {
  task_type = "telemetry_data_cleanup"
}

resource "mistershell_scheduled_task_config" "cleanup" {
  id = data.mistershell_scheduled_task_config.cleanup.id
  configuration = {
    is_enabled       = true
    interval_minutes = 60
    timeout_seconds  = 300
  }
}
