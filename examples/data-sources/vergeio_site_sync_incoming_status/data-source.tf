# Fail the plan when this incoming sync has not run, or last synced more
# than max_lag_seconds ago. site_id and name are the other way to look it up.

data "vergeio_site_sync_incoming_status" "example" {
  site_id         = vergeio_site.example.id
  name            = vergeio_site_sync_incoming.example.name
  max_lag_seconds = 3600
}
