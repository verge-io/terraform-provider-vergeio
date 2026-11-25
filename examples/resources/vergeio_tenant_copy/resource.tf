# Copy a tenant's current state and keep the copy in state.
# This is not a snapshot clone. Tenants.Clone has no snapshot argument.
# isolate posts isolateon on the copy. Destroy deletes the copy.

resource "vergeio_tenant_copy" "example" {
  tenant_id = vergeio_tenant.source.id
  name      = "pr-128"
  isolate   = true
}
