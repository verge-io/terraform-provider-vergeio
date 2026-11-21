resource "vergeio_vm_import" "guest" {
  name = "migrated-guest"
  url  = "https://downloads.example.com/images/guest.ova"

  timeouts {
    create = "20m"
  }
}

resource "vergeio_vm_import" "disk" {
  name   = "golden"
  target = "media"
  url    = "https://downloads.example.com/images/golden.qcow2"
}

resource "vergeio_vm" "workload" {
  name      = "workload"
  os_family = "linux"
  cpu_cores = 1
  ram       = 1024

  boot_disk {
    name   = "os"
    media  = "import"
    source = vergeio_vm_import.disk.file_id
  }
}

data "vergeio_vm_import" "from_ansible" {
  name = "pipeline-guest"
}
