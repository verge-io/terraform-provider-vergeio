# A NAS service on an existing virtual machine. user blocks are the accounts
# on that service. password_wo is sent when the user is created and when
# password_wo_version changes. Terraform stores the version, not the password.

resource "vergeio_vm" "nas" {
  name       = "Example NAS"
  enabled    = true
  cpu_cores  = 2
  ram        = 2048
  powerstate = false
}

resource "vergeio_nas_service" "example" {
  vm_id                 = vergeio_vm.nas.id
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
