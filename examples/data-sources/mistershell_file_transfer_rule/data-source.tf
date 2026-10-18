data "mistershell_file_transfer_rule" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_file_transfer_rule" "by_name" {
  name = "file-transfer-rule"
}
