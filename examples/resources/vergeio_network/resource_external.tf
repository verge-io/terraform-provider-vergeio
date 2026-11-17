# Create a layer 2 external vNET with a VLAN tag and bonding.

resource "vergeio_network" "external_example" {
  name           = "External Example"
  enabled        = true
  interface_vnet = 4
  layer2_id      = 1000
  type           = "external"
  on_power_loss  = "last_state"
  ipaddress_type = "none"
  layer2_type    = "vlan"

  enable_bonding       = true
  bond_interfaces_args = [4, 5]
}
