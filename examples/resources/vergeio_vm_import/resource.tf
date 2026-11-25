# Import an OVA as a virtual machine. Create waits until VergeOS
# reports complete or warning. A failed import returns immediately.
# Destroy removes the import row and the VM it created.

resource "vergeio_vm_import" "example" {
  name           = "imported-guest"
  url            = "https://downloads.example.com/images/guest.ova"
  preserve_macs  = false
  preferred_tier = "3"

  timeouts {
    create = "20m"
  }
}
