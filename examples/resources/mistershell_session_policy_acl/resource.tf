# An ACL combining a glob pattern and a regex pattern. Pattern order is
# significant (evaluation merges them in order).
resource "mistershell_session_policy_acl" "dangerous_commands" {
  name        = "dangerous-commands"
  description = "Blocks destructive shell commands"

  patterns = [
    {
      pattern = "rm -rf *"
      type    = "glob" # default; may be omitted
    },
    {
      pattern = "^sudo\\s+"
      type    = "regex"
    },
  ]
}

# A disabled ACL with a single glob pattern.
resource "mistershell_session_policy_acl" "package_managers" {
  name    = "package-managers"
  enabled = false

  patterns = [
    {
      pattern = "apt*"
      type    = "glob"
    },
  ]
}
