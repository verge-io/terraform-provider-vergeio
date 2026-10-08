# Snapshot a tenant before a change inside it. The snapshot is not Terraform state.
# The vergeio_tenant_snapshot resource keeps a snapshot. This action does not.
# terraform apply -invoke=action.vergeio_tenant_snapshot.before runs it on its own.
# Requires Terraform 1.14. OpenTofu does not implement actions.

resource "vergeio_tenant" "customer" {
  name       = "customer-a"
  password   = "change-me"
  powerstate = false

  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_tenant_snapshot.before]
    }
  }
}

action "vergeio_tenant_snapshot" "before" {
  config {
    tenant_id         = vergeio_tenant.customer.id
    name              = "before-change"
    description       = "Snapshot taken before the tenant update"
    retention_seconds = 86400
  }
}
