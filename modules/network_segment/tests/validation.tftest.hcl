mock_provider "vergeio" {}

variables {
  name        = "app"
  cidr        = "192.168.16.0/24"
  dhcp_start  = "192.168.16.10"
  dhcp_stop   = "192.168.16.200"
  dns_servers = ["192.168.16.1"]
}

run "cidr_must_be_network_address" {
  command = plan

  variables {
    cidr = "192.168.16.5/24"
  }

  expect_failures = [
    var.cidr,
  ]
}

run "firewall_action" {
  command = plan

  variables {
    firewall_rules = [{
      name   = "bad"
      action = "nat"
    }]
  }

  expect_failures = [
    var.firewall_rules,
  ]
}

run "dhcp_range_required" {
  command = plan

  variables {
    dhcp_start = null
    dhcp_stop  = null
  }

  expect_failures = [
    terraform_data.check,
  ]
}
