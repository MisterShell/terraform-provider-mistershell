# AI prompts are imported by their integer ID.
# Note: variable_schema is stored from your configuration and is NOT set on
# import (only id is). Re-supply variable_schema in your configuration after
# importing so the next plan does not show a diff.
terraform import mistershell_ai_prompt.example 42
