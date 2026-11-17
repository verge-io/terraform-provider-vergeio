# Stage several rules, then refresh the network once from the last rule.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_rule" "http" {
  vnet              = vergeio_network.lan.id
  name              = "allow-http"
  protocol          = "tcp"
  destination_ports = "80"
  apply             = false
}

resource "vergeio_network_rule" "https" {
  vnet              = vergeio_network.lan.id
  name              = "allow-https"
  protocol          = "tcp"
  destination_ports = "443"

  depends_on = [vergeio_network_rule.http]
}
