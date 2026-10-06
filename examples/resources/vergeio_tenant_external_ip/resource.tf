# Hand the tenant a virtual IP from the parent external network.
# The first address VergeOS assigns becomes the tenant UI address.
# Set vergeio_tenant.ui_address_id to this resource's id on a later change
# when a different assigned IP should be the UI address.
# Changing tenant_id, network_id, ip, hostname, or description replaces the address.
# apply_parent_firewall applies the parent network rules in the same call.
# parent_firewall_pending stays true while that network still needs its rules applied.

resource "vergeio_tenant_external_ip" "example" {
  tenant_id             = vergeio_tenant.example.id
  network_id            = vergeio_network.external.id
  ip                    = "203.0.113.50"
  hostname              = "customer-a"
  description           = "Tenant UI address"
  apply_parent_firewall = true
}
