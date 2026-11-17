# Kill a tenant node that will not stop. reset restarts it instead.
# Neither operation changes vergeio_tenant_node.
# Power off for maintenance is a VergeOS tenant node action the pinned
# client does not expose, so it is not offered here.
# terraform apply -invoke=action.vergeio_tenant_node_power.stop runs it on its own.
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

action "vergeio_tenant_node_power" "stop" {
  config {
    tenant_node_id = vergeio_tenant_node.node1.id
    operation      = "kill"
  }
}
