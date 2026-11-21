# Export configuration for one NAS volume. This does not start an export.
# Destroy removes the configuration and leaves files already on the volume.

resource "vergeio_vm_export" "example" {
  volume         = "0123456789abcdef0123456789abcdef01234567"
  quiesced       = true
  create_current = true
  max_exports    = 3
}
