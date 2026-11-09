resource "mistershell_snap_customizations" "collection" {
  id = mistershell_snap_config.configuration.id
  configuration = {
    items = [{
      tag_id          = mistershell_tag.production.id
      skip_collection = true
    }]
  }
}
