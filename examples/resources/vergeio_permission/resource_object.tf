# Grant one user read and modify on a single VM.
# Rights are separate booleans. They are not a bit mask.

resource "vergeio_permission" "web_vm" {
  user_id   = vergeio_user.operator.id
  table     = "vms"
  object_id = 123
  list      = true
  read      = true
  create    = false
  modify    = true
  delete    = false
}

resource "vergeio_user" "operator" {
  name     = "operator"
  enabled  = true
  password = "change-me"
}
