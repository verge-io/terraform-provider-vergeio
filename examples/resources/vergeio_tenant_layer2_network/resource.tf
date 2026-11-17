# Bridge the tenant onto a parent layer 2 network.
# Requires VergeOS 26.0 or later.
# Changing tenant_id or network_id replaces the assignment.
# enabled updates in place and defaults to true.
# Destroy disables the assignment, then deletes it.
# Networks created inside the tenant remain after the host-side delete.
# They belong to the tenant-side configuration.
# Leaving those components in place can block a later recreation.

resource "vergeio_tenant_layer2_network" "example" {
  tenant_id  = vergeio_tenant.example.id
  network_id = vergeio_network.external.id
  enabled    = true
}
