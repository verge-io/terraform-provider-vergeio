# Download a disk image into the media catalog. VergeOS rejects this
# file on vm_imports. Attach it with vergeio_vm_drive media = "import".

resource "vergeio_vm_import" "disk" {
  name      = "golden"
  target    = "media"
  url       = "https://downloads.example.com/images/golden.qcow2"
  file_name = "golden.qcow2"

  timeouts {
    create = "20m"
  }
}
