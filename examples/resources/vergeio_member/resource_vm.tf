# Add a VM to a group.

resource "vergeio_member" "vm_membership" {
  group  = 5
  member = "vms/123"
}
