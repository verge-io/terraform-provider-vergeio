# Incoming sync on a site. site_id is vergeio_site.id.
# registration_code is filled by VergeOS after create.

resource "vergeio_site" "example" {
  name    = "Remote Lab"
  url     = "https://203.0.113.10"
  enabled = false
}

resource "vergeio_site_sync_incoming" "example" {
  site_id       = vergeio_site.example.id
  name          = "From Remote Lab"
  description   = "Copies received from the remote system"
  min_snapshots = 2
  enabled       = true
}
