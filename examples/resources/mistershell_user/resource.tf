# A local user. password is required (the API requires it for every auth type).
# Changing password rotates it via the admin reset endpoint; changing email or
# auth_type replaces the user.
resource "mistershell_user" "alice" {
  email      = "alice@example.com"
  first_name = "Alice"
  last_name  = "Anderson"
  password   = "change-me-please"
  is_active  = true
}

# An externally-authenticated user. A password is still required by the API even
# for external auth types (its complexity is not enforced for non-LOCAL users).
# Such users are normally provisioned at login, but the API permits creating
# them directly.
resource "mistershell_user" "oidc_bob" {
  email      = "bob@example.com"
  first_name = "Bob"
  last_name  = "Baker"
  auth_type  = "OIDC"
  password   = "change-me-please"
}
