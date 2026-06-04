# Credentials are imported by their integer ID.
# Note: credential_data secret values are NOT read back from the server
# (the API masks them); set credential_data in config after importing.
terraform import mistershell_credential.example 789
