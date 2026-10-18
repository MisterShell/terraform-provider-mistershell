resource "mistershell_location" "site" {
  name      = "File storage site"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_file_store" "example" {
  name        = "site-files"
  backend     = "local"
  location_id = mistershell_location.site.id
}
