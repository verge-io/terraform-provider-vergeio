# A volume on a NAS service. Destroy disables the volume and waits until
# VergeOS allows the delete.

resource "vergeio_vm" "nas" {
  name       = "Example NAS"
  enabled    = true
  cpu_cores  = 2
  ram        = 2048
  powerstate = false
}

resource "vergeio_nas_service" "example" {
  vm_id = vergeio_vm.nas.id
}

resource "vergeio_nas_volume" "example" {
  service_id  = vergeio_nas_service.example.id
  name        = "documents"
  description = "Shared documents"
  enabled     = true
}
