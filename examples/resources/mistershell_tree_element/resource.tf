resource "mistershell_tree_element" "example" {
  parent_key   = "location:2"
  element_type = "label"
  label        = "Production"
  config = {
    icon    = "shield"
    color   = "warning"
    variant = "badge"
  }
}
