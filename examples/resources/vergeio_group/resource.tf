# Create a group, put a user in it, and grant the group rights on every VM.
# vergeio_member.group is a number. The group id is the string key VergeOS assigned.

resource "vergeio_group" "operators" {
  name        = "operators"
  description = "Operators"
  enabled     = true
}

resource "vergeio_user" "operator" {
  name            = "operator"
  displayname     = "Operator"
  email           = "operator@example.com"
  enabled         = true
  type            = "normal"
  password        = "change-me"
  change_password = true
}

resource "vergeio_member" "operator" {
  group  = tonumber(vergeio_group.operators.id)
  member = format("users/%s", vergeio_user.operator.id)
}

resource "vergeio_permission" "vms" {
  group_id = vergeio_group.operators.id
  table    = "vms"
  list     = true
  read     = true
  create   = true
  modify   = false
  delete   = false
}
