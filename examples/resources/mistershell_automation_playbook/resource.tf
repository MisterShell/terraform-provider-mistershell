
resource "mistershell_automation_playbook" "example" {
  name        = "recorded-session-workflow"
  description = "A disabled workflow ready for actions."
  enabled     = false
  graph = {
    trigger = { event_name = "resource.session.recorded" }
    nodes   = []
    edges   = []
  }
}
