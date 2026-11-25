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

resource "vergeio_network" "lan" {
  name        = "tf-acc-vm-lan"
  description = "tf-acc-vm-lan"
  type        = "internal"
  network     = "192.168.88.0/24"
  ipaddress   = "192.168.88.1"
  powerstate  = false
}

resource "vergeio_tag_category" "role" {
  name         = "tf-acc-role"
  description  = "tf-acc-role"
  taggable_vms = true
}

resource "vergeio_tag" "web" {
  category    = tonumber(vergeio_tag_category.role.id)
  name        = "tf-acc-web"
  description = "tf-acc-web"
}

module "vm" {
  source = "../.."

  name           = "tf-acc-vm"
  description    = "tf-acc-vm"
  source_type    = "image"
  boot_disk_size = 8
  powerstate     = false
  cpu_cores      = 1
  ram            = 1024

  nics = [{
    name      = "tf-acc-lan"
    vnet      = tonumber(vergeio_network.lan.id)
    interface = "virtio"
  }]

  tag_ids = {
    web = tonumber(vergeio_tag.web.id)
  }
}

output "vm_id" {
  value = module.vm.id
}

output "network_id" {
  value = vergeio_network.lan.id
}
