resource "mistershell_snap_config" "configuration" {
  id = var.snap_key
  configuration = {
    is_enabled    = true
    display_order = 10
    options       = {}
  }
}
