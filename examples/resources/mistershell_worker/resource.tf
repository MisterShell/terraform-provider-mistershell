# A worker hanging under a location, with task-handler config and a config schema.
resource "mistershell_location" "emea" {
  name      = "EMEA"
  parent_id = 1
}

resource "mistershell_worker" "example" {
  name        = "emea-worker-1"
  description = "Worker serving the EMEA location"
  location_id = mistershell_location.emea.id
  is_enabled  = true

  # Opaque, free-form task-handler config pushed to the worker via heartbeat.
  # Keys are read by task handlers with get_config_value(key, default); the only
  # backend-recognized key today is manual_worker_presence_ip (admin override for
  # the IP shown in the UI). Everything else is consumed by your task handlers.
  config = jsonencode({
    manual_worker_presence_ip = "203.0.113.10"
  })

  # Optional JSON Schema the backend uses to validate the config above.
  config_schema = jsonencode({
    type = "object"
    properties = {
      manual_worker_presence_ip = { type = "string" }
    }
    additionalProperties = true
  })
}

# The authentication token is returned ONLY at creation time and can never be
# read back afterwards. It is stored in Terraform state (sensitive). If lost,
# regenerate it out of band (POST /workers/{id}/regenerate-token).
output "worker_token" {
  value     = mistershell_worker.example.token
  sensitive = true
}
