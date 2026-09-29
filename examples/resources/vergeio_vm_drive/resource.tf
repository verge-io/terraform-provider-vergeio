# Attach drives to a VM this configuration does not have to be the only owner of.

resource "vergeio_vm" "web-server" {
  name       = "my-web-server"
  enabled    = true
  os_family  = "linux"
  cpu_cores  = 2
  ram        = 2048
  powerstate = false

  boot_disk {
    name = "os"
    size = 40
  }
}

resource "vergeio_vm_drive" "data" {
  vm_id          = vergeio_vm.web-server.id
  name           = "data"
  description    = "Data disk"
  disksize       = 100
  interface      = "virtio-scsi"
  preferred_tier = 3
  orderid        = 1
}

resource "vergeio_vm_drive" "cdrom" {
  vm_id        = vergeio_vm.web-server.id
  name         = "CD ROM"
  description  = "CD ROM"
  media        = "cdrom"
  media_source = 33
  interface    = "ahci"
}

resource "vergeio_vm_drive" "clone" {
  vm_id          = vergeio_vm.web-server.id
  name           = "Clone"
  description    = "Clone"
  media          = "clone"
  media_source   = 39
  preferred_tier = 3
  interface      = "virtio-scsi"
}
