# Workers are imported by their integer ID.
# Note: token, config, and config_schema are NOT read back on import. The token
# can never be recovered from the server (regenerate it out of band if lost),
# and config / config_schema are stored-from-config — re-supply all three in
# your configuration after importing.
terraform import mistershell_worker.example 123
