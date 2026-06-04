# Session-policy ACLs are imported by their integer ID.
# Note: built-in ACLs (is_builtin = true) cannot be edited or deleted; do not
# manage them with Terraform.
terraform import mistershell_session_policy_acl.dangerous_commands 42
