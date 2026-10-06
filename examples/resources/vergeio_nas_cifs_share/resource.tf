# A CIFS share on a volume. User and host lists put one entry on each line.

resource "vergeio_network" "nas" {
  name         = "Example NAS Net"
  type         = "internal"
  enabled      = true
  network      = "192.168.8.0/24"
  ipaddress    = "192.168.8.1"
  dhcp_enabled = true
  dhcp_start   = "192.168.8.10"
  dhcp_stop    = "192.168.8.200"
}

resource "vergeio_nas_service" "example" {
  name       = "Example NAS"
  network_id = vergeio_network.nas.id

  user {
    name                = "files"
    password_wo         = "changeme!"
    password_wo_version = 1
  }
}

resource "vergeio_nas_volume" "example" {
  service_id = vergeio_nas_service.example.id
  name       = "documents"
  max_size   = 1048576
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
