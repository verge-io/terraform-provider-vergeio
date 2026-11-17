# Move a tenant node onto another host before that host is maintained.
# target_node is the host node key. vergeio_tenant_node is not updated.
# terraform apply -invoke=action.vergeio_tenant_node_migrate.before_host_work runs it on its own.
# Requires Terraform 1.14. OpenTofu does not implement actions.

resource "vergeio_tenant" "customer" {
  name                = "customer-a"
  description         = "Customer A virtual data center"
  password_wo         = "change-me"
  password_wo_version = 1
  powerstate          = true
}

resource "vergeio_tenant_node" "node1" {
  tenant_id = vergeio_tenant.customer.id
  name      = "node1"
  cpu_cores = 4
  ram       = 8192
  enabled   = true
}

action "vergeio_tenant_node_migrate" "before_host_work" {
  config {
    tenant_node_id = vergeio_tenant_node.node1.id
    target_node    = "3"
  }
}
