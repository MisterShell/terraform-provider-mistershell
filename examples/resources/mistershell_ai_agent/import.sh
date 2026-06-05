# AI agents are imported by their integer ID.
# Note: `config` is stored from configuration and is NOT read back from the
# server on import (the backend may enrich/reorder the blob). Only `id` and the
# other server-managed fields are populated on import — re-supply the full
# `config` in your configuration after importing.
# Only user agents (type "chat" or "background") can be imported and managed;
# builtin_* agents are managed by MisterShell (use the data source for those).
terraform import mistershell_ai_agent.example 123
