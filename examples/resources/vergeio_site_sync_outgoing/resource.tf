# Outgoing sync. period.profile_period is a snapshot profile period key.
# retention is how long the remote copy is kept. It does not change the
# local profile. registration_code_wo is sent only when the sync is created.

resource "vergeio_snapshot_profile" "cloud" {
  name        = "Cloud Snapshots"
  description = "Schedule used for cloud snapshots and this sync"

  period {
    name      = "nightly"
    frequency = "daily"
    hour      = 1
    minute    = 0
    retention = 604800
    immutable = true
  }
}

resource "vergeio_site" "example" {
  name    = "Remote Lab"
  url     = "https://203.0.113.10"
  enabled = false
}

resource "vergeio_site_sync_outgoing" "example" {
  site_id     = vergeio_site.example.id
  name        = "To Remote Lab"
  description = "Nightly copies"
  enabled     = false
  note        = "Remote retention is one week"

  registration_code_wo         = "example-registration-code"
  registration_code_wo_version = 1

  period {
    profile_period     = vergeio_snapshot_profile.cloud.period[0].key
    retention          = 604800
    priority           = 5
    do_not_expire      = true
    destination_prefix = "dr-"
  }
}
