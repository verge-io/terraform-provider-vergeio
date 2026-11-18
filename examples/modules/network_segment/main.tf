terraform {
  required_version = ">= 1.11.0"

  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "vergeio" {}

variable "external_network_name" {
  type        = string
  description = "Name of the external network used as the outbound gateway. Empty skips that gateway."
  default     = "External"
}

data "vergeio_networks" "external" {
  count       = var.external_network_name == "" ? 0 : 1
  filter_name = var.external_network_name
  filter_type = "external"
}

module "app" {
  source = "../../../modules/network_segment"

  name                = "app"
  description         = "Application segment"
  cidr                = "192.168.16.0/24"
  ipaddress           = "192.168.16.1/24"
  powerstate          = true
  dhcp_start          = "192.168.16.10"
  dhcp_stop           = "192.168.16.200"
  dns_servers         = ["192.168.16.1"]
  domain              = "app.example.com"
  external_network_id = var.external_network_name == "" ? null : data.vergeio_networks.external[0].networks[0].id

  dns = {
    zone_domain = "app.example.com"
    email       = "hostmaster@example.com"
    recursion   = true
    records = [{
      host  = "www"
      type  = "A"
      value = "192.168.16.10"
    }]
  }

  firewall_rules = [{
    name              = "allow-ssh"
    description       = "SSH from management"
    protocol          = "tcp"
    source_ip         = "192.0.2.0/24"
    destination_ports = "22"
  }]
}

output "network_id" {
  description = "Internal network key."
  value       = module.app.id
}

output "external_network_id" {
  description = "Outbound gateway key written to vnet_default_gateway."
  value       = module.app.external_network_id
}
