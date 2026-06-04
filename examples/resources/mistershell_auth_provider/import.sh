# Auth providers are imported by their integer ID.
# Note: config is NOT read back from the server (the API masks secrets and
# enriches non-secret defaults); after importing, set config in your
# configuration yourself. config is excluded from import.
terraform import mistershell_auth_provider.corp_oidc 3
