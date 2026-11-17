# One DNS view. apply defaults to true and refreshes DNS after the change.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_dns_view" "internal" {
  network_id    = vergeio_network.lan.id
  name          = "internal"
  match_clients = "10.0.0.0/8;"
  recursion     = true
}
