mock_provider "vergeio" {}

variables {
  name                = "app"
  description         = "Application segment"
  cidr                = "192.168.16.0/24"
  ipaddress           = "192.168.16.1/24"
  enabled             = true
  powerstate          = true
  dhcp_enabled        = true
  dynamic_dhcp        = true
  dhcp_sequential     = true
  dhcp_start          = "192.168.16.10"
  dhcp_stop           = "192.168.16.200"
  dns_servers         = ["192.168.16.1", "192.0.2.53"]
  domain              = "app.example.com"
  external_network_id = 3

  dns = {
    zone_domain   = "app.example.com"
    view_name     = "internal"
    match_clients = "192.168.16.0/24;"
    recursion     = true
    email         = "hostmaster@example.com"
    records = [{
      host        = "www"
      type        = "A"
      value       = "192.168.16.10"
      ttl         = "1h"
      description = "Application"
    }]
  }

  firewall_rules = [
    {
      name              = "allow-ssh"
      description       = "SSH from management"
      protocol          = "tcp"
      direction         = "incoming"
      action            = "accept"
      source_ip         = "192.0.2.0/24"
      destination_ports = "22"
    },
    {
      name              = "fwd-https"
      protocol          = "tcp"
      action            = "translate"
      destination_ports = "443"
      target_ip         = "192.168.16.10"
      target_ports      = "443"
    },
  ]
}

run "plan" {
  command = plan

  assert {
    condition     = vergeio_network.this.name == "app" && vergeio_network.this.type == "internal" && vergeio_network.this.network == "192.168.16.0/24"
    error_message = "The internal network CIDR was not planned."
  }

  assert {
    condition     = vergeio_network.this.dhcp_enabled == true && vergeio_network.this.dynamic_dhcp == true && vergeio_network.this.dhcp_sequential == true && vergeio_network.this.dhcp_start == "192.168.16.10" && vergeio_network.this.dhcp_stop == "192.168.16.200"
    error_message = "DHCP was not planned."
  }

  assert {
    condition     = vergeio_network.this.dnslist == "192.168.16.1,192.0.2.53" && vergeio_network.this.domain == "app.example.com"
    error_message = "DHCP DNS servers and domain were not planned."
  }

  assert {
    condition     = vergeio_network.this.vnet_default_gateway == 3
    error_message = "The external network was not planned as vnet_default_gateway."
  }

  assert {
    condition     = vergeio_network_rules.this.rule[0].name == "allow-ssh" && vergeio_network_rules.this.rule[0].action == "accept" && vergeio_network_rules.this.rule[1].action == "translate" && vergeio_network_rules.this.rule[1].target_ip == "192.168.16.10"
    error_message = "The firewall policy was not planned."
  }

  assert {
    condition     = vergeio_network_dns_view.this[0].name == "internal" && vergeio_network_dns_view.this[0].apply == false && vergeio_network_dns_zone.this[0].domain == "app.example.com" && vergeio_network_dns_zone.this[0].type == "master" && vergeio_network_dns_record.this["www/A/192.168.16.10"].value == "192.168.16.10"
    error_message = "The DNS view, zone, or record was not planned."
  }
}

run "no_outbound_gateway" {
  command = plan

  variables {
    external_network_id = null
  }

  assert {
    condition     = vergeio_network.this.vnet_default_gateway == null
    error_message = "vnet_default_gateway should stay unset when external_network_id is omitted."
  }
}

run "dhcp_dns_without_zone" {
  command = plan

  variables {
    dns = null
  }

  assert {
    condition     = length(vergeio_network_dns_zone.this) == 0 && vergeio_network.this.dnslist == "192.168.16.1,192.0.2.53"
    error_message = "Omitting dns should skip the zone and still hand DNS servers to DHCP clients."
  }
}
