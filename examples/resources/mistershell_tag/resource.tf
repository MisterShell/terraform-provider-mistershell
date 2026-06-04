# A tag with a custom color and a description.
resource "mistershell_tag" "production" {
  name        = "production"
  color       = "#e53935"
  description = "Production network resources"
}

# A tag that owns the full set of resources it is assigned to.
# Managing resource_ids takes exclusive ownership of the tag's membership:
# Terraform replaces the whole set on every change.
resource "mistershell_tag" "core_switches" {
  name        = "core-switches"
  color       = "blue"
  description = "Core switching layer"

  resource_ids = [
    mistershell_resource.spine1.id,
    mistershell_resource.spine2.id,
  ]
}
