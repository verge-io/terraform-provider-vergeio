# Give the tenant a node. tenant_id is the key of vergeio_tenant.
# ram is megabytes and must be at least 2048.

resource "vergeio_tenant_node" "example" {
  tenant_id     = vergeio_tenant.example.id
  name          = "node1"
  cpu_cores     = 4
  ram           = 8192
  enabled       = true
  on_power_loss = "last_state"
}
