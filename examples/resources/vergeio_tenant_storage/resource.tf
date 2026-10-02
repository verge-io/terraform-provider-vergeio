# Allocate 100 GiB from storage tier 1. provisioned is bytes and must be a whole GiB
# (multiples of 1073741824). tier is the storage tier key. Changing tier replaces the allocation.

resource "vergeio_tenant_storage" "example" {
  tenant_id   = vergeio_tenant.example.id
  tier        = 1
  provisioned = 107374182400
}
