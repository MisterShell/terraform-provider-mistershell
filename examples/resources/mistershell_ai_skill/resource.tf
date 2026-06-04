# A custom AI skill: a markdown platform brief surfaced to agents via the
# list_skills tool. The body is plain markdown and may contain Jinja2 template
# variables ({{ ... }}) that the backend substitutes at prep time.
#
# agent_types restricts which agent types may discover this skill. resource_types
# is intentionally omitted (not set to []) so the skill is offered for any
# resource type — passing an empty list would be converted to null server-side
# and show as drift on the next plan.
resource "mistershell_ai_skill" "runbook" {
  name        = "incident-triage-runbook"
  description = "First-response triage steps for on-call agents."
  is_enabled  = true

  agent_types = ["chat"]

  body = <<-EOT
    # Incident Triage Runbook

    When asked to triage an incident on resource {{ resource_name }}:

    1. Confirm the resource is reachable and note its health status.
    2. Pull the last 15 minutes of events; correlate by timestamp.
    3. Summarize the single most likely root cause before suggesting any fix.

    Never inject a write or destructive command — hand it to the operator to run.
  EOT
}

# A second skill scoped by resource_types so it is only offered to agents
# operating on Linux hosts. resource_types values are resource-type descriptor
# keys; they are a discovery filter, NOT an authorization gate.
resource "mistershell_ai_skill" "linux_disk_pressure" {
  name        = "linux-disk-pressure-brief"
  description = "How to investigate disk pressure on Linux hosts."
  is_enabled  = true

  agent_types    = ["chat"]
  resource_types = ["linux"]

  body = <<-EOT
    # Linux Disk Pressure

    Start with `df -h` and `du -xhd1 /` on the largest mount. Check for
    deleted-but-open files (`lsof +L1`) and journald/log growth under /var/log.
    Report the top offenders before recommending cleanup.
  EOT
}
