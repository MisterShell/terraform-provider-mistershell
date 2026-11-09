data "mistershell_canary_alert_rule" "fallback" {
  name = "Log all Canary interactions"
}

resource "mistershell_canary_rule_order" "example" {
  # Include every existing rule. This example assumes only these two rules exist.
  ordered_ids = [
    tonumber(mistershell_canary_alert_rule.example.id),
    tonumber(data.mistershell_canary_alert_rule.fallback.id),
  ]
}
