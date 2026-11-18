# Lab apply test. terraform test does not load this subdirectory.
# Run it against a lab with:
#   terraform test -test-directory=tests/lab

provider "vergeio" {}

variables {
  name                = "tf-mod-segment"
  cidr                = "192.168.16.0/24"
  ipaddress           = "192.168.16.1/24"
  dhcp_start          = "192.168.16.10"
  dhcp_stop           = "192.168.16.200"
  dns_servers         = ["192.168.16.1"]
  domain              = "app.example.com"
  external_network_id = 3
  powerstate          = false

  dns = {
    zone_domain = "app.example.com"
    email       = "hostmaster@example.com"
  }

  firewall_rules = [{
    name              = "allow-ssh"
    protocol          = "tcp"
    destination_ports = "22"
  }]
}

run "apply" {
  command = apply

  assert {
    condition     = vergeio_network.this.id != ""
    error_message = "Lab apply did not store a network id."
  }
}
