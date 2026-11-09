resource "mistershell_note" "example" {
  scope_type = "location"
  scope_id   = 2
  content    = "# Operations\nManaged operations instructions."
}
# Destroy relinquishes management and preserves the note and its history.
