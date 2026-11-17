# Hand the tenant a routed CIDR from the parent external network.
# The tenant can build a network on this range.
# Changing tenant_id, network_id, cidr, or description replaces the block.
# apply_parent_firewall applies the parent network rules in the same call.
# parent_firewall_pending stays true while that network still needs its rules applied.
# VergeOS refuses a delete while a tenant network is still built on this block.

resource "vergeio_tenant_network_block" "example" {
  tenant_id             = vergeio_tenant.example.id
  network_id            = vergeio_network.external.id
  cidr                  = "198.51.100.0/28"
  description           = "Customer routed addresses"
  apply_parent_firewall = true
}
