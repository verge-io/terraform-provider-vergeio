# tenant

Creates a VergeOS tenant on the parent system, with nodes, storage, routed network blocks, external IPs, and a parent-system admin user.

`tenant_password_wo` is the built-in tenant admin password (`vergeio_tenant.password_wo`). It is not stored. Increment `tenant_password_wo_version` to change it. `admin_user` is a separate `vergeio_user` on the parent, added to `admin_group_id` with `vergeio_member`. `admin_password_wo` is that user's password and is not stored.

The first external IP becomes `ui_address`. A stack that manages the inside of the tenant uses that address as the provider host. This module configures the parent system. A second configuration, with its host set to `ui_address`, manages the inside of the tenant.

`provisioned` is bytes and must be a positive multiple of 1073741824 (1 GiB). `apply_parent_firewall` defaults to true on blocks and addresses.

```hcl
module "tenant" {
  source = "../../modules/tenant"

  name                       = "customer-a"
  tenant_password_wo         = var.tenant_password
  tenant_password_wo_version = 1
  admin_group_id            = data.vergeio_groups.admins.groups[0].id
  admin_password_wo         = var.admin_password
  admin_password_wo_version = 1

  admin_user = {
    name = "customer-a-admin"
  }

  nodes = [{
    name      = "node1"
    cpu_cores = 4
    ram       = 8192
  }]

  storage = [{
    tier        = 1
    provisioned = 107374182400
  }]

  network_blocks = [{
    network_id = vergeio_network.external.id
    cidr       = "198.51.100.0/28"
  }]

  external_ips = [{
    network_id = vergeio_network.external.id
    ip         = "203.0.113.50"
    hostname   = "customer-a"
  }]
}
```

A runnable root module is in `examples/modules/tenant`.

`terraform test` in this directory plans with a mocked provider. Lab apply tests are in `tests/lab` and are not part of that run:

```bash
terraform test -test-directory=tests/lab
```
