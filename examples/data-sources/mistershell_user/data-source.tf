# Look up a user by ID.
data "mistershell_user" "by_id" {
  id = 123
}

# Look up a user by email (must match exactly one user).
data "mistershell_user" "by_email" {
  email = "alice@example.com"
}
