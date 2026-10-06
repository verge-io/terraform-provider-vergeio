# Take a snapshot before a VM update. The snapshot is not Terraform state.
# terraform apply -invoke=action.vergeio_vm_snapshot.before runs it on its own.
# Requires Terraform 1.14. OpenTofu does not implement actions.

resource "vergeio_vm" "web" {
  name        = "web"
  description = "updated"

  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_vm_snapshot.before]
    }
  }
}

action "vergeio_vm_snapshot" "before" {
  config {
    vm_id             = vergeio_vm.web.id
    name              = "before-change"
    retention_seconds = 86400
  }
}
