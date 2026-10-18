
resource "mistershell_fact_check" "example" {
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
