# Grant a group list and read on every VM. Omit object_id for a table grant.
# VergeOS stores that grant with row 0.

resource "vergeio_permission" "all_vms" {
  group_id = vergeio_group.operators.id
  table    = "vms"
  list     = true
  read     = true
  create   = false
  modify   = false
  delete   = false
}

resource "vergeio_group" "operators" {
  name        = "operators"
  description = "Operators"
  enabled     = true
}
