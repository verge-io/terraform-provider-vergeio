# Own every non-system firewall rule on a network, in order.
# Do not also use vergeio_network_rule on this network.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_rules" "lan" {
  vnet = vergeio_network.lan.id

  rule = [
    {
      name              = "allow-ssh"
      protocol          = "tcp"
      direction         = "incoming"
      action            = "accept"
      source_ip         = "alias:mgmt-nets"
      destination_ports = "22"
    },
    {
      name              = "fwd-app-tls"
      protocol          = "tcp"
      action            = "translate"
      destination_ports = "8443"
      target_ip         = "10.0.0.15"
      target_ports      = "443"
    },
  ]
}
