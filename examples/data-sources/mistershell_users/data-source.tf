# List every user.
data "mistershell_users" "all" {}

# Narrow the list with a substring filter on name/email.
data "mistershell_users" "active_admins" {
  search = "admin"
}
