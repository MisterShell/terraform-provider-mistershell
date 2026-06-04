# Look up a worker by its integer ID.
data "mistershell_worker" "by_id" {
  id = 123
}

# Look up a worker by its exact name.
data "mistershell_worker" "by_name" {
  name = "emea-worker-1"
}

output "worker_status" {
  value = data.mistershell_worker.by_name.status
}

output "worker_location_id" {
  value = data.mistershell_worker.by_name.location_id
}
