# password_wo is the tenant admin password. Terraform stores
# password_wo_version, not the password. Increment the version to
# change it. The tenant is updated in place.

resource "vergeio_tenant" "example" {
  name                = "customer-a"
  description         = "Customer A virtual data center"
  password_wo         = "change-me"
  password_wo_version = 1
  powerstate          = false
}
