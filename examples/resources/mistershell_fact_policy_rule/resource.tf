resource "mistershell_fact_check" "ntp" {
  name        = "ntp-synchronized"
  description = "Check NTP synchronization."
  fact_type   = "system_ntp"
  assertions = [{
    id               = "synchronized"
    description      = "NTP must be synchronized"
    match_mode       = "exists"
    field_conditions = [{ field = "sync_status", operator = "eq", value = "synchronized" }]
  }]
}

resource "mistershell_fact_policy_rule" "example" {
  name      = "ntp-policy"
  enabled   = false
  check_ids = [tonumber(mistershell_fact_check.ntp.id)]
  match     = "pass"
}
