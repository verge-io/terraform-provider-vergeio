# Shut the guest down without changing vergeio_vm.powerstate.
# A later plan of the VM restores a declared powerstate.
# terraform apply -invoke=action.vergeio_vm_power.maintenance runs it on its own.
# Requires Terraform 1.14. OpenTofu does not implement actions.

action "vergeio_vm_power" "maintenance" {
  config {
    vm_id           = vergeio_vm.web.id
    operation       = "shutdown"
    timeout_seconds = 120
    force           = false
  }
}

resource "vergeio_vm" "web" {
  name       = "web"
  powerstate = true
}
