# Log destinations are imported by their integer ID.
# Note: webhook auth secrets (bearer token, basic password, header value) are
# NOT read back from the server (the API masks them as "****"); set the full
# config in your configuration after importing.
terraform import mistershell_log_destination.example 123
