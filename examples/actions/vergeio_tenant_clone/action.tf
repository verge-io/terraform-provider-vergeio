# Copy a tenant into a new name. The copy is not Terraform state.
# no_vnet, no_storage, and no_nodes skip those parts. Omit a flag to copy it.
# terraform apply -invoke=action.vergeio_tenant_clone.sandbox runs it on its own.
# Requires Terraform 1.14. OpenTofu does not implement actions.

resource "vergeio_tenant" "template" {
  name                = "customer-template"
  description         = "Tenant used as a clone source"
  password_wo         = "change-me"
  password_wo_version = 1
  powerstate          = false
}

action "vergeio_tenant_clone" "sandbox" {
  config {
    tenant_id  = vergeio_tenant.template.id
    name       = "sandbox-a"
    no_vnet    = false
    no_storage = false
    no_nodes   = false
  }
}
