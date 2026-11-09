data "mistershell_builtin_sensor_source" "feed" {
  name = "et/open"
}

resource "mistershell_builtin_sensor_source" "feed" {
  id            = data.mistershell_builtin_sensor_source.feed.id
  configuration = { enabled = true }
}
