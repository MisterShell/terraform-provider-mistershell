# Create a location hierarchy: EMEA > Switzerland > Zurich
resource "mistershell_location" "emea" {
  name        = "EMEA"
  kind        = "geo"
  description = "Europe, Middle East, and Africa"
  parent_id   = 1 # MisterShell root location
}

resource "mistershell_location" "switzerland" {
  name      = "Switzerland"
  kind      = "geo"
  parent_id = mistershell_location.emea.id
}

resource "mistershell_location" "zurich" {
  name        = "Zurich DC"
  kind        = "geo"
  description = "Zurich data center"
  parent_id   = mistershell_location.switzerland.id
  latitude    = 47.3769
  longitude   = 8.5417
}
