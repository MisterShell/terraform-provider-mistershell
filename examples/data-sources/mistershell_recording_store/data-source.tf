data "mistershell_recording_store" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_recording_store" "by_name" {
  name = "recording-store"
}
