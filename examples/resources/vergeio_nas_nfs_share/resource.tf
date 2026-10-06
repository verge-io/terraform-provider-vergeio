# An NFS share on a volume. allowed_hosts is a comma separated list.

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
  service_id = vergeio_nas_service.example.id
  name       = "documents"
}

resource "vergeio_nas_nfs_share" "example" {
  volume_id     = vergeio_nas_volume.example.id
  name          = "documents"
  description   = "Documents share"
  allowed_hosts = "10.0.0.0/8,192.0.2.0/24"
  squash        = "root_squash"
  data_access   = "rw"
  enabled       = true
}
