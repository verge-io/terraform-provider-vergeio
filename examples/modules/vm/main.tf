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

variable "image_name" {
  type        = string
  description = "Media image name. vergeio_mediasources returns its id for the boot disk."
}

variable "network_name" {
  type        = string
  description = "Network name for the VM NIC."
}

data "vergeio_mediasources" "image" {
  filter_name = var.image_name
}

data "vergeio_networks" "lan" {
  filter_name = var.network_name
}

resource "vergeio_tag_category" "role" {
  name         = "role"
  taggable_vms = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "vergeio_tag" "web" {
  category    = tonumber(vergeio_tag_category.role.id)
  name        = "web"
  description = "Web servers"
}

module "web" {
  source = "../../../modules/vm"

  name            = "web1"
  description     = "Web server"
  source_type     = "image"
  media_source_id = data.vergeio_mediasources.image.mediasources[0].id
  boot_disk_size  = 40
  cpu_cores       = 2
  ram             = 2048
  machine_type    = "q35"
  guest_agent     = true
  powerstate      = false

  cloudinit_user_data = <<-EOT
    #cloud-config
    hostname: web1
  EOT

  nics = [{
    name             = "lan"
    vnet             = data.vergeio_networks.lan.networks[0].id
    interface        = "virtio"
    assign_ipaddress = true
  }]

  tag_ids = {
    web = tonumber(vergeio_tag.web.id)
  }

  snapshot_profile = {
    name        = "web-nightly"
    description = "Nightly snapshots for web1"
    periods = [{
      name      = "nightly"
      frequency = "daily"
      hour      = 2
      minute    = 0
      retention = 604800
      quiesce   = true
    }]
  }
}

output "vm_id" {
  description = "VM key."
  value       = module.web.id
}

output "snapshot_profile_id" {
  description = "Snapshot profile key created for this VM."
  value       = module.web.snapshot_profile_id
}
