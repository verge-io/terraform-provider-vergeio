# A NAS service from the Services recipe. name is the virtual machine name.
# network_id is the network that machine joins. user blocks are the accounts
# on that service. password_wo is sent when the user is created and when
# password_wo_version changes. Terraform stores the version, not the password.

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
  name                  = "Example NAS"
  network_id            = vergeio_network.nas.id
  max_imports           = 4
  max_syncs             = 2
  disable_swap          = false
  read_ahead_kb_default = "128"

  user {
    name                = "files"
    password_wo         = "changeme"
    password_wo_version = 1
    display_name        = "Files"
    description         = "Opens the documents share"
    enabled             = true
    home_drive          = "H"
  }
}
