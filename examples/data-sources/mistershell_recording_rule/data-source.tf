data "mistershell_recording_rule" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_recording_rule" "by_name" {
  name = "recording-rule"
}
