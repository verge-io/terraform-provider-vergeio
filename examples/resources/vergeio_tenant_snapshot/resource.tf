# Keep a snapshot of a tenant. name can be omitted and VergeOS will assign one.
# Changing name, tenant_id, or type replaces the snapshot. description and the
# expiration update in place. The vergeio_tenant_snapshot action takes a
# snapshot and does not keep it in state.

resource "vergeio_tenant_snapshot" "example" {
  tenant_id   = vergeio_tenant.example.id
  name        = "before-change"
  description = "Taken before the maintenance window"
  type        = "full"
  expires     = 1893456000
}

resource "vergeio_tenant_snapshot" "keep" {
  tenant_id     = vergeio_tenant.example.id
  name          = "keep"
  description   = "Kept until it is deleted"
  never_expires = true
}
