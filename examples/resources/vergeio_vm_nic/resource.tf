# Attach a NIC to a VM. Omit enabled to leave the VergeOS default, which is enabled.

resource "vergeio_vm" "web-server" {
  name       = "my-web-server"
  enabled    = true
  cpu_cores  = 2
  ram        = 2048
  powerstate = false

  boot_disk {
    name = "os"
    size = 40
  }
}

resource "vergeio_vm_nic" "web" {
  vm_id            = vergeio_vm.web-server.id
  name             = "Web Server Network"
  description      = "NIC for Web Server"
  interface        = "virtio"
  vnet             = 6
  assign_ipaddress = true
}
