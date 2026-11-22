# Copy tenant 7 and keep the copy in state.
# snapshot_id must be a snapshot of that tenant. The copy is the current
# tenant: Tenants.Clone has no snapshot argument. isolate posts isolateon
# on the copy. Destroy deletes the copy.

resource "vergeio_tenant_copy" "example" {
  tenant_id   = vergeio_tenant.source.id
  snapshot_id = "8"
  name        = "pr-128"
  isolate     = true
}
