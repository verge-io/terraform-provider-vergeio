# Drives other than boot_disk are vergeio_vm_drive. NICs are vergeio_vm_nic.

terraform {
  required_providers {
    vergeio = {
      source = "vergeio/cloud/vergeio"
    }
  }
}

# VERGEOS_HOST, VERGEOS_API_KEY or VERGEOS_USERNAME and VERGEOS_PASSWORD
provider "vergeio" {
  host     = "vergeos.example.com"
  username = "admin"
  password = "password"
  insecure = false
}

resource "vergeio_vm" "example" {
  name         = "example"
  description  = "Example VM"
  enabled      = true
  os_family    = "linux"
  cpu_cores    = 4
  machine_type = "q35"
  ram          = 8192

  boot_disk {
    name = "os"
    size = 40
  }
}

resource "vergeio_vm_drive" "data" {
  vm_id    = vergeio_vm.example.id
  name     = "data"
  disksize = 100
}

resource "vergeio_vm_nic" "lan" {
  vm_id = vergeio_vm.example.id
  name  = "lan"
  vnet  = 6
}
