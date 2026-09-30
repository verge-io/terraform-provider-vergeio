# One firewall rule. apply defaults to true and refreshes the network after the change.
# Do not use vergeio_network_rule on a network that also has vergeio_network_rules.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_rule" "ssh" {
  vnet              = vergeio_network.lan.id
  name              = "allow-ssh"
  protocol          = "tcp"
  direction         = "incoming"
  action            = "accept"
  source_ip         = "192.0.2.0/24"
  destination_ports = "22"
}
