# Lab apply test. terraform test does not load this subdirectory.
# Run it against a lab with:
#   terraform test -test-directory=tests/lab

provider "vergeio" {}

variables {
  name        = "tf-acc-segment"
  description = "tf-acc-segment"
  cidr        = "192.168.77.0/24"
  dhcp_start  = "192.168.77.10"
  dhcp_stop   = "192.168.77.200"
  dns_servers = ["192.168.77.1"]
  domain      = "tf-acc.example.com"
  powerstate  = false

  dns = {
    view_name   = "tf-acc-view"
    zone_domain = "tf-acc.example.com"
    email       = "hostmaster@example.com"
    records = [{
      host  = "tf-acc-www"
      type  = "A"
      value = "192.168.77.10"
    }]
  }

  firewall_rules = [{
    name              = "tf-acc-allow-ssh"
    protocol          = "tcp"
    destination_ports = "22"
  }]
}

run "apply" {
  command = apply

  assert {
    condition     = vergeio_network.this.id != "" && vergeio_network.this.name == "tf-acc-segment"
    error_message = "Lab apply did not store the tf-acc-segment network."
  }
}
