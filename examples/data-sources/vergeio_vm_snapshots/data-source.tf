data "vergeio_vm_snapshots" "source" {
  vm_id = vergeio_vm.source.id
}

output "newest_snapshot" {
  value = data.vergeio_vm_snapshots.source.snapshots[length(data.vergeio_vm_snapshots.source.snapshots) - 1].id
}
