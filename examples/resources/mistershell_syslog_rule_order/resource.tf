resource "mistershell_syslog_rule_order" "example" {
  # Include all existing rule IDs, including the server's fallback policy.
  ordered_ids = var.all_syslog_rule_ids_in_order
}
