# Restart a tenant without changing vergeio_tenant.
# A later plan restores a declared powerstate when the running state differs.
# terraform apply -invoke=action.vergeio_tenant_reset.restart runs it on its own.
# Requires Terraform 1.14. OpenTofu does not implement actions.

resource "vergeio_tenant" "customer" {
  name                = "customer-a"
  description         = "Customer A virtual data center"
  password_wo         = "change-me"
  password_wo_version = 1
  powerstate          = true
}

action "vergeio_tenant_reset" "restart" {
  config {
    tenant_id = vergeio_tenant.customer.id
  }
}
