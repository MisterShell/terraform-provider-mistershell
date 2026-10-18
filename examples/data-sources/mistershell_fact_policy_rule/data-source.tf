data "mistershell_fact_policy_rule" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_fact_policy_rule" "by_name" {
  name = "fact-policy-rule"
}
