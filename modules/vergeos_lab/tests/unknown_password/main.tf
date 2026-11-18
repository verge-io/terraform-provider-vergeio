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

# The password and the network key are unknown until apply. The install seed
# is still a fixed two-file list, which is what vergeio_vm.cloudinit_files can plan.
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

resource "terraform_data" "network" {}

module "lab" {
  source = "../.."

  name                = "lab1"
  vergeos_version     = "26.0.2"
  admin_password      = random_password.admin.result
  admin_email         = "admin@example.com"
  external_network_id = length(terraform_data.network.id)
  external_ip         = "198.51.100.10"
  external_prefix     = 24
  external_gateway    = "198.51.100.1"
  external_dns        = "198.51.100.1"
  cpu_cores           = 1
  ram                 = 1024
  tier0_disk_count    = 1
  tier1_disk_count    = 1
  tier0_disk_size_gb  = 8
  tier1_disk_size_gb  = 8
}

output "vm_names" {
  value = module.lab.vm_names
}
