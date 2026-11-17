# ipsec_id references the IPsec configuration, which references the network.
# Destroy deletes this phase 2, then this phase 1, then the IPsec
# configuration, then the network.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_ipsec" "lan" {
  network_id = vergeio_network.lan.id
}

resource "vergeio_network_ipsec_connection" "branch" {
  ipsec_id       = vergeio_network_ipsec.lan.id
  name           = "branch-office"
  remote_gateway = "203.0.113.10"
  keyexchange    = "ikev2"
  auth           = "psk"
  ike            = "aes256-sha256-modp2048"
  psk            = "replace-with-a-long-preshared-key"
  auto           = "start"

  phase2 {
    name     = "lan-to-lan"
    local    = "192.168.0.0/24"
    remote   = "198.51.100.0/24"
    protocol = "esp"
    ciphers  = "aes128-sha256-modp2048,aes128gcm128-sha256-modp2048"
  }
}
