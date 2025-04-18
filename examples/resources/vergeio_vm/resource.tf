
# Create a VM with a drive and a nic
resource "vergeio_vm" "web-server" {
  name                 = "my-web-server"
  description          = "Web Server"
  enabled              = true
  os_family            = "linux"
  cpu_cores            = 2
  machine_type         = "q35"
  ram                  = 2048
  powerstate           = false
  guest_agent          = true
  cloudinit_datasource = "nocloud"
  ha_group             = "web"

  # Drive 
  vergeio_drive {
    name           = "Web Server OS Disk"
    description    = "Operating System Disk"
    disksize       = 10
    interface      = "virtio-scsi"
    preferred_tier = 3
    orderid        = 0
  }

  # NIC
  vergeio_nic {
    name             = "Web Server Network"
    description      = "NIC for Web Server"
    interface        = "virtio"
    enabled          = true
    assign_ipaddress = true
  }
}
