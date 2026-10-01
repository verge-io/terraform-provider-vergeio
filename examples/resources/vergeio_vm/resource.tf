# Create a VM with a boot disk, devices, and cloud-init files.
# Extra drives are vergeio_vm_drive resources. NICs are vergeio_vm_nic resources.

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
  on_power_loss        = "last_state"
  cloudinit_datasource = "nocloud"
  ha_group             = "web"

  boot_disk {
    name = "Web Server OS Disk"
    size = 100
  }

  vergeio_device {
    name           = "my-vgpu"
    description    = "my-vgpu"
    type           = "node_nvidia_vgpu_devices"
    resource_group = "1aeba871-c293-d325-cc15-188183d13cc3"
    nvidia_vgpu_settings = {
      profile_type       = "profile identifier"
      frame_rate_limiter = 60
      disable_vnc        = true
      enable_debugging   = true
      enable_uvm         = true
      enable_profiling   = true
    }
  }

  vergeio_device {
    name           = "my-PCI-device"
    description    = "my-PCI-device"
    type           = "node_pci_devices"
    resource_group = "22c12938-05f0-310f-f00e-df6f109e7364"
  }

  vergeio_device {
    name        = "my-TPM-device"
    description = "my-TPM-device"
    type        = "tpm"
    tpm_settings = {
      model   = "crb"
      version = "2.0" # display label; sent on create as "2". Read-only after create.
    }
  }

  vergeio_device {
    name        = "my-USB-device"
    description = "my-USB-device"
    type        = "node_usb_devices"
    usb_settings = {
      guest_reset      = false
      guest_resets_all = false
    }
  }

  cloudinit_files = [
    {
      name     = "user-data"
      contents = <<-EOT
        #cloud-config
        users:
            - name: ubuntu
            groups: sudo
            shell: /bin/bash
            sudo: ["ALL=(ALL) NOPASSWD:ALL"]
            ssh_authorized_keys:
                - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD...
      EOT
    },
    {
      name     = "meta-data"
      contents = <<-EOT
        instance-id: iid-local01
        local-hostname: myhost
      EOT
    }
  ]
}
