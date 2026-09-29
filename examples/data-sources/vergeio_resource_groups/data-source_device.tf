data "vergeio_resource_groups" "vgpu" {
  filter_name = "vGPU Pool"
}

resource "vergeio_vm" "gpu_vm" {
  name         = "gpu-workstation"
  cpu_cores    = 8
  ram          = 32768
  machine_type = "q35"
  os_family    = "linux"

  vergeio_device {
    name           = "vgpu-device"
    description    = "Nvidia vGPU"
    type           = "node_nvidia_vgpu_devices"
    resource_group = data.vergeio_resource_groups.vgpu.resource_groups[0].id
    nvidia_vgpu_settings = {
      profile_type       = "grid_p40-4q"
      frame_rate_limiter = 60
    }
  }
}
