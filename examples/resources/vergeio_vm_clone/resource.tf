# Clone the snapshot VM for machine_snapshots key 9 onto network 6.
# The live VM is not the clone target. Destroy deletes the copy.

resource "vergeio_vm_clone" "example" {
  snapshot_id = "9"
  name        = "pr-128-api"
  network_id  = vergeio_network.sandbox.id
}
