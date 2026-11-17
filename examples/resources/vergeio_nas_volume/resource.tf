# A volume on a NAS service. Destroy disables the volume and waits until
# VergeOS allows the delete.

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
  service_id  = vergeio_nas_service.example.id
  name        = "documents"
  description = "Shared documents"
  enabled     = true
  max_size    = 1048576
}
