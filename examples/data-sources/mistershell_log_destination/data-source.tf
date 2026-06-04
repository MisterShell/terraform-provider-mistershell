# Look up a log destination by its integer ID.
data "mistershell_log_destination" "by_id" {
  id = 123
}

# Look up a log destination by its exact name.
data "mistershell_log_destination" "by_name" {
  name = "central-syslog"
}

output "destination_type" {
  value = data.mistershell_log_destination.by_name.type
}

output "destination_streams" {
  value = data.mistershell_log_destination.by_name.streams
}
