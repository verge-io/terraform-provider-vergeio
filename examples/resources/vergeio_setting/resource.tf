# max_connections is a setting VergeOS already stores.
# Destroy sets this key back to its default and leaves other settings alone.

resource "vergeio_setting" "example" {
  key   = "max_connections"
  value = "200"
}
