# One IPsec configuration per network. network_id references the network, so
# Terraform deletes this configuration before the network. Delete connections
# first: phase 2, then phase 1.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_ipsec" "lan" {
  network_id = vergeio_network.lan.id
  enabled    = true
  mode       = "normal"
}
