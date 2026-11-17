# Fail the plan when this outgoing sync has not run, or last ran more
# than max_lag_seconds ago.

data "vergeio_site_sync_outgoing_status" "example" {
  id              = vergeio_site_sync_outgoing.example.id
  max_lag_seconds = 3600
}
