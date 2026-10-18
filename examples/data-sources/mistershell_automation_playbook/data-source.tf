data "mistershell_automation_playbook" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_automation_playbook" "by_name" {
  name = "automation-playbook"
}
