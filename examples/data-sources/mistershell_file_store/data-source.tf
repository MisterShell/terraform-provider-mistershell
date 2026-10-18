data "mistershell_file_store" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_file_store" "by_name" {
  name = "file-store"
}
