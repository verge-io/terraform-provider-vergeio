# Own every non-system firewall rule on a network, in order.
# Do not also use vergeio_network_rule on this network.
# Reference an alias by id: VergeOS resolves alias:<key>, not alias:<name>.

resource "vergeio_network" "lan" {
  name = "Example LAN"
}

resource "vergeio_network_rule_alias" "mgmt" {
  name             = "mgmt-nets"
  description      = "Management networks"
  value            = "192.0.2.0/24|Management,198.51.100.0/24|Lab"
  publishing_scope = "private"
}

resource "vergeio_network_rules" "lan" {
  vnet = vergeio_network.lan.id

  rule = [
    {
      name              = "allow-ssh"
      protocol          = "tcp"
      direction         = "incoming"
      action            = "accept"
      source_ip         = "alias:${vergeio_network_rule_alias.mgmt.id}"
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
