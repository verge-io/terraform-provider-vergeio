# wireguard_id references the interface, so this peer is destroyed first.
# configure_firewall defaults to site-to-site. apply refreshes the network.

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

resource "vergeio_network_wireguard_peer" "office" {
  wireguard_id = vergeio_network_wireguard.wg0.id
  name         = "remote-office"
  peer_ip      = "192.168.255.2"
  public_key   = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
  allowed_ips  = "10.0.0.0/24"
  endpoint     = "vpn.example.com"
  port         = 51820
  keepalive    = 25
}
