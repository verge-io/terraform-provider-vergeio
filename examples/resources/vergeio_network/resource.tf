


# Create Network with all parameters

resource "vergeio_network" "example" {
  name                 = "Example Net"
  enabled              = true
  vnet_default_gateway = 3
  network              = "192.168.0.0/24"
  dhcp_enabled         = true
  dhcp_sequential      = true
  dynamic_dhcp         = true
  dhcp_start           = "192.168.0.2"
  dhcp_stop            = "192.168.0.200"
  on_power_loss        = "power_on"
}
