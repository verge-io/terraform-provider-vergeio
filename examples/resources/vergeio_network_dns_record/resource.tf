# One A record. value is the address stored in the zone.

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
}

resource "vergeio_network_dns_record" "www" {
  zone_id = vergeio_network_dns_zone.example.id
  host    = "www"
  type    = "A"
  value   = "192.0.2.10"
}
