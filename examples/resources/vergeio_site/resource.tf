# A remote VergeOS system. Create does not build syncs.
# auth_password_wo is sent on create and when the version changes.
# Terraform stores the version, not the password.

resource "vergeio_site" "example" {
  name        = "Remote Lab"
  url         = "https://203.0.113.10"
  description = "Paired system in the other building"
  city        = "Austin"
  enabled     = false

  config_cloud_snapshots = "disabled"

  auth_user                = "sync"
  auth_password_wo         = "example-password"
  auth_password_wo_version = 1
}
