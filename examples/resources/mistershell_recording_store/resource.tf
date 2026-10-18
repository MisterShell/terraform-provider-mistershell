resource "mistershell_credential" "storage" {
  name            = "recording-storage"
  credential_type = "aws_credentials"
  credential_data = { access_key = var.aws_access_key, secret_key = var.aws_secret_key }
}

resource "mistershell_recording_store" "example" {
  name          = "session-recordings"
  backend       = "s3"
  credential_id = mistershell_credential.storage.id
  params = {
    s3_bucket = "company-session-recordings"
    s3_region = "eu-west-1"
  }
}
