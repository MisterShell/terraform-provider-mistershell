
resource "mistershell_report_template" "example" {
  name               = "inventory-posture"
  report_type        = "inventory_posture"
  access_mode        = "public"
  ttl_hours          = 24
  ai_summary_enabled = false
}
