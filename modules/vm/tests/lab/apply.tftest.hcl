# Lab apply test. terraform test does not load this subdirectory.
# Run it against a lab with:
#   terraform test -test-directory=tests/lab
#
# media_source_id and vnet must exist on that lab. Replace the defaults
# before applying.

provider "vergeio" {}

variables {
  name            = "tf-mod-vm"
  source_type     = "image"
  media_source_id = 1
  powerstate      = false

  nics = [{
    name             = "lan"
    vnet             = 1
    interface        = "virtio"
    assign_ipaddress = true
  }]

  tags = [{
    category = "role"
    name     = "tf-mod-vm"
  }]
}

run "apply" {
  command = apply

  assert {
    condition     = vergeio_vm.image[0].id != ""
    error_message = "Lab apply did not store a VM id."
  }
}
