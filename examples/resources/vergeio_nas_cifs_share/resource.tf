# A CIFS share on a volume. User and host lists put one entry on each line.

resource "vergeio_vm" "nas" {
  name       = "Example NAS"
  enabled    = true
  cpu_cores  = 2
  ram        = 2048
  powerstate = false
}

resource "vergeio_nas_service" "example" {
  vm_id = vergeio_vm.nas.id

  user {
    name                = "files"
    password_wo         = "changeme"
    password_wo_version = 1
  }
}

resource "vergeio_nas_volume" "example" {
  service_id = vergeio_nas_service.example.id
  name       = "documents"
}

resource "vergeio_nas_cifs_share" "example" {
  volume_id   = vergeio_nas_volume.example.id
  name        = "documents"
  description = "Documents share"
  comment     = "Documents"
  valid_users = "files"
  browseable  = true
  enabled     = true
}
