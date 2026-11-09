resource "mistershell_file_catalog_folder" "runbooks" {
  location_id = 2
  name        = "runbooks"
}

resource "mistershell_file_catalog_file" "example" {
  location_id    = mistershell_file_catalog_folder.runbooks.location_id
  parent_path    = mistershell_file_catalog_folder.runbooks.path
  name           = "operations.md"
  content_type   = "text/markdown"
  content_base64 = base64encode("# Operations\nContact the operations team.\n")
  # Use filebase64("${path.module}/operations.md") for a local file.
}
