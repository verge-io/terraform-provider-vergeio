terraform {
  required_version = ">= 1.11.0"

  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "random" {}

provider "vergeio" {}

resource "random_password" "admin" {
  length           = 24
  upper            = true
  lower            = true
  numeric          = true
  special          = true
  override_special = "!@#%^*-_"
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
}

locals {
  external_cidr = "10.221.0.0/24"
}

resource "vergeio_network" "external" {
  name        = "tf-acc-vergeos-lab-ext"
  description = "tf-acc-vergeos-lab-ext"
  type        = "internal"
  network     = local.external_cidr
  ipaddress   = cidrhost(local.external_cidr, 1)
  powerstate  = false
}

module "lab" {
  source = "../.."

  name                = "tf-acc-vergeos-lab"
  shape               = "hci"
  vergeos_version     = "26.0.0"
  admin_password      = random_password.admin.result
  admin_email         = "tf-acc-vergeos-lab@example.com"
  external_network_id = tonumber(vergeio_network.external.id)
  external_ip         = cidrhost(local.external_cidr, 10)
  external_prefix     = 24
  external_gateway    = cidrhost(local.external_cidr, 1)
  external_dns        = cidrhost(local.external_cidr, 1)
  core_fabric_1_cidr  = "10.221.1.0/24"
  core_fabric_2_cidr  = "10.221.2.0/24"
  powerstate          = false
  network_powerstate  = false
  cpu_cores           = 1
  ram                 = 1024
  tier0_disk_count    = 1
  tier1_disk_count    = 1
  tier0_disk_size_gb  = 8
  tier1_disk_size_gb  = 8
}

output "vm_ids" {
  value = module.lab.vm_ids
}

output "vm_names" {
  value = module.lab.vm_names
}

output "core_network_ids" {
  value = module.lab.core_network_ids
}

output "external_network_id" {
  value = vergeio_network.external.id
}

output "vergeos_version" {
  value = module.lab.vergeos_version
}
