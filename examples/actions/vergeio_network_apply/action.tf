# Stage a rule without refreshing the network, then apply the staged rules.
# terraform apply -invoke=action.vergeio_network_apply.rules runs the refresh
# on its own. Requires Terraform 1.14. OpenTofu does not implement actions.

resource "vergeio_network" "lan" {
  name       = "Example LAN"
  powerstate = true
}

resource "vergeio_network_rule" "ssh" {
  vnet              = vergeio_network.lan.id
  name              = "allow-ssh"
  protocol          = "tcp"
  direction         = "incoming"
  action            = "accept"
  source_ip         = "192.0.2.0/24"
  destination_ports = "22"
  apply             = false

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.vergeio_network_apply.rules]
    }
  }
}

action "vergeio_network_apply" "rules" {
  config {
    network_id = vergeio_network.lan.id
    target     = "rules"
  }
}
