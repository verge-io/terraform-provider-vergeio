# apply defaults to true and refreshes the network so the Accept WireGuard
# rules VergeOS stages are not left unapplied. network_id references the
# network, so this interface is destroyed before the network.

resource "vergeio_network" "lan" {
  name       = "Example LAN"
  powerstate = true
}

resource "vergeio_network_wireguard" "wg0" {
  network_id  = vergeio_network.lan.id
  name        = "wg0"
  ip          = "192.168.255.1/24"
  listen_port = 51820
}
