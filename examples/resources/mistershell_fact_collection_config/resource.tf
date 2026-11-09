resource "mistershell_fact_collection_config" "collection" {
  id = var.fact_collection_key
  configuration = {
    is_enabled    = true
    display_order = 10
  }
}
