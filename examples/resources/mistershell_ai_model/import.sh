# AI models are imported by their integer ID.
# Note: config secrets (api_key and any key named secret/secret_key/token/
# password/access_key) are NOT read back from the server (the API masks them as
# "***"); re-supply the full config in your configuration after importing.
terraform import mistershell_ai_model.example 123
