# An NFS share on a volume. allowed_hosts is a comma separated list.

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
}

resource "vergeio_nas_volume" "example" {
  service_id = vergeio_nas_service.example.id
  name       = "documents"
  max_size   = 1048576
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
