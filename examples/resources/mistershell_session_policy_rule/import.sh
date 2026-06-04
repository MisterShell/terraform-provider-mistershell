# Session-policy rules are imported by their integer ID.
# Note: the last remaining rule cannot be deleted (the rule chain is never
# emptied); edit it instead of destroying it.
terraform import mistershell_session_policy_rule.deny_dangerous 7
