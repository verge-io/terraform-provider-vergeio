# A named address group. Rules reference it as alias:${vergeio_network_rule_alias.mgmt.id}.

resource "vergeio_network_rule_alias" "mgmt" {
  name             = "mgmt-nets"
  description      = "Management networks"
  value            = "192.0.2.0/24|Management,198.51.100.0/24|Lab"
  publishing_scope = "private"
}
