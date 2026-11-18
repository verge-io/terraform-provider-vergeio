# Lab apply test. terraform test does not load this subdirectory.
# Run it against a lab with:
#   terraform test -test-directory=tests/lab

provider "vergeio" {}

variables {
  name                       = "tf-mod-tenant"
  description                = "Module lab tenant"
  tenant_password_wo         = "tenant-admin-password"
  tenant_password_wo_version = 1
  powerstate                 = false
  admin_group_id             = 1
  admin_password_wo          = "parent-admin-password"

  admin_user = {
    name = "tf-mod-tenant-admin"
  }

  nodes = [{
    name      = "node1"
    cpu_cores = 2
    ram       = 2048
  }]

  storage = [{
    tier        = 1
    provisioned = 1073741824
  }]

  network_blocks = [{
    network_id = "3"
    cidr       = "198.51.100.0/28"
  }]

  external_ips = [{
    network_id = "3"
    ip         = "203.0.113.50"
    hostname   = "tf-mod-tenant"
  }]
}

run "apply" {
  command = apply

  assert {
    condition     = vergeio_tenant.this.id != "" && vergeio_user.admin.id != ""
    error_message = "Lab apply did not store a tenant id and an admin user id."
  }
}
