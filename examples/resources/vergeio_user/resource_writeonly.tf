# password_wo is not stored. Increment password_wo_version to change it.
# Changing password_wo without changing the version does not update the user.

resource "vergeio_user" "testuser" {
  name                = "testuser1"
  displayname         = "testuser1"
  email               = "testuser@verge.io"
  enabled             = true
  type                = "normal"
  password_wo         = "changeme123"
  password_wo_version = 1
  change_password     = true
}
