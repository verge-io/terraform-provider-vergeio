# One DNS zone in a view.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_dns_view" "internal" {
  network_id = vergeio_network.lan.id
  name       = "internal"
}

resource "vergeio_network_dns_zone" "example" {
  view_id = vergeio_network_dns_view.internal.id
  domain  = "example.com"
  type    = "master"
  email   = "hostmaster.example.com"
}
