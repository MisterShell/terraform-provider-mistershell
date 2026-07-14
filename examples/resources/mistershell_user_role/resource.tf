# Assign the "auditor" role to Alice. Role assignments are managed exclusively
# through this edge resource; mistershell_user exposes assigned roles read-only.
resource "mistershell_user_role" "alice_auditor" {
  user_id = mistershell_user.alice.id
  role_id = mistershell_role.auditor.id
}
