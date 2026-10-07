# Separate example from vm.tf. Deploy a workload VM from a media image.
# vergeio_mediasources.id is the value boot_disk.source sends as media_source.

data "vergeio_mediasources" "golden" {
  filter_name = "golden"
}

resource "vergeio_vm" "web" {
  name      = "web1"
  os_family = "linux"
  cpu_cores = 2
  ram       = 2048

  boot_disk {
    name   = "os"
    media  = "import"
    source = data.vergeio_mediasources.golden.mediasources[0].id
  }
}
