# Custom AI skills are imported by their integer ID. Unlike opaque JSON fields
# elsewhere in this provider, all attributes round-trip on import: name, body,
# description, agent_types, resource_types, and is_enabled are all read back
# from the server.
#
# Note: builtin skills (is_builtin = true) are managed by MisterShell. They
# cannot be created or deleted by this resource, and only their is_enabled flag
# is mutable; importing one would let Terraform plan a destroy it cannot perform.
# Read builtin skills through the mistershell_ai_skill data source instead.
terraform import mistershell_ai_skill.example 42
