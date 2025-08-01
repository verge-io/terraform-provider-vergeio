# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers {
    vergeio = {
      source = "vergeio/cloud/vergeio"
      #version = "1.5.3"
    }
  }
}
provider "vergeio" {
  host     = "131.153.164.250"
  username = "admin"
  password = "P@ssw0rd!"
  insecure = true
}


# # resource "vergeio_network" "web-app_internal_network" {
# #   count                = var.vm_count
# #   name                 = "Network_${count.index + 1}"
# #   enabled              = true
# #   vnet_default_gateway = 3
# #   on_power_loss        = "power_on"
# # }

# resource "vergeio_vm" "web-app" {
#   count                = var.vm_count
#   name                 = "${var.vm_name_prefix}-${count.index + 1}"
#   description          = "Web App VM TEST1 ${count.index + 1}"
#   enabled              = true
#   os_family            = "linux"
#   cpu_cores            = var.cpu_cores
#   machine_type         = "q35"
#   ram                  = var.ram
#   powerstate           = false
#   guest_agent          = true
#   cloudinit_datasource = "nocloud"
#   ha_group             = "prod"

#   # Create a new drive
#   vergeio_drive {
#     name           = "New Disk"
#     description    = "New Disk"
#     disksize       = 110
#     interface      = "virtio-scsi"
#     preferred_tier = 3
#     orderid        = 0
#   }

# vergeio_drive {
#   name           = "New Disk 1"
#   description    = "New Disk 1"
#   disksize       = 10
#   interface      = "virtio-scsi"
#   preferred_tier = 3
#   orderid        = 1
# }

#   # vergeio_drive {
#   #   name           = "New Disk 2"
#   #   description    = "New Disk 2"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 2
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 3"
#   #   description    = "New Disk 3"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 3
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 4"
#   #   description    = "New Disk 4"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 4
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 5"
#   #   description    = "New Disk 5"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 5
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 6"
#   #   description    = "New Disk 6"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 6
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 7"
#   #   description    = "New Disk 7"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 7
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 8"
#   #   description    = "New Disk 8"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 8
#   # }

#   # vergeio_drive {
#   #   name           = "New Disk 9"
#   #   description    = "New Disk 9"
#   #   disksize       = 10
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   orderid        = 9
#   # }
#   # vergeio_drive {
#   #   # count          =  var.vm_count
#   #   name           = "OS disk ${count.index + 1}"
#   #   description    = "OS disk for Web App VM ${count.index + 1}"
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   enabled        = true
#   #   disksize       = 22
#   # }

#   # Import an image from media 
#   # vergeio_drive {
#   #   name           = "Imported Disk"
#   #   description    = "Imported Disk from media images"
#   #   media          = "import"
#   #   media_source   = 4
#   #   preferred_tier = 3
#   #   interface      = "virtio-scsi"
#   # }

#   # EFI Drive
#   # vergeio_drive {
#   #   name        = "EFI Disk"
#   #   description = "New EFI Disk"
#   #   media       = "efidisk"
#   # }

#   # # Clone a drive
#   # vergeio_drive {
#   #   name           = "Cloned 1 Disk"
#   #   description    = "clone of existing disk"
#   #   media          = "clone"
#   #   media_source   = 3
#   #   preferred_tier = 3
#   #   interface      = "virtio-scsi"
#   # }

#   # Mount a CDROM
#   # vergeio_drive {
#   #   name         = "cdrom"
#   #   media        = "cdrom"
#   #   media_source = 4
#   # }




#   # vergeio_drive {
#   #   # count          =  var.vm_count
#   #   name           = "OS disk ${count.index + 2}"
#   #   description    = "OS disk for Web App VM ${count.index + 2}"
#   #   interface      = "virtio-scsi"
#   #   preferred_tier = 3
#   #   enabled        = true
#   #   disksize       = 30
#   # }


#   vergeio_nic {
#     name        = "Network 1"
#     description = "Network for Web App VM 1"
#     interface   = "virtio"
#     enabled     = true
#   }

#   vergeio_nic {
#     name        = "Network 2"
#     description = "Network for Web App VM 2"
#     interface   = "virtio"
#     enabled     = true
#   }

#   vergeio_nic {
#     name        = "Network 3"
#     description = "Network for Web App VM 3"
#     interface   = "virtio"
#     enabled     = true
#   }
# }
# #   # vergeio_nic {
#   #   # count          =  var.vm_count
#   #   name        = "Network 2 ${count.index + 1}"
#   #   description = "Network 2 for Web App VM ${count.index + 1}"
#   #   interface   = "virtio"
#   #   enabled     = true
#   # }
# }

# #   cloudinit_files = [
# #     {
# #       name     = "user-data"
# #       contents = <<-EOT
# #         #cloud-config
# #         users:
# #             - name: ubuntu
# #             groups: sudo
# #             shell: /bin/bash
# #             sudo: ["ALL=(ALL) NOPASSWD:ALL"]
# #             ssh_authorized_keys:
# #                 - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD...
# #         EOT
# #     },
# #     {
# #       name     = "meta-data"
# #       contents = <<-EOT
# #         instance-id: iid-local01
# #         local-hostname: myhost
# #         EOT
# #     }
# #   ]
# # }



