# Protection policy for a VM. retention is seconds and is required.
# There is no default. An omitted retention is not keep-forever and is not
# 24 hours. vergeio_vm.snapshot_profile is the profile key.

resource "vergeio_snapshot_profile" "nightly" {
  name        = "nightly"
  description = "Nightly and weekly VM snapshots"

  period {
    name      = "nightly"
    frequency = "daily"
    hour      = 2
    minute    = 0
    retention = 604800
    quiesce   = true
  }

  period {
    name        = "weekly"
    frequency   = "weekly"
    day_of_week = "sun"
    hour        = 1
    minute      = 0
    retention   = 2419200
    quiesce     = true
  }
}

resource "vergeio_vm" "web" {
  name             = "web"
  snapshot_profile = tonumber(vergeio_snapshot_profile.nightly.id)
}
