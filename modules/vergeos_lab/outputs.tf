output "vm_ids" {
  description = "VM keys, keyed by node. controller-1 creates the nested system."
  value       = { for key, vm in vergeio_vm.node : key => vm.id }
}

output "vm_names" {
  description = "VM names."
  value       = { for key, vm in vergeio_vm.node : key => vm.name }
}

output "core_network_ids" {
  description = "Core fabric network keys."
  value = {
    core_1 = vergeio_network.core_1.id
    core_2 = vergeio_network.core_2.id
  }
}

output "ui_address" {
  description = "External address written into the controller seed. Nothing checks that a nested API is listening there."
  value       = var.external_ip
}

output "vergeos_version" {
  description = "VergeOS version recorded on the nodes and in the seed."
  value       = var.vergeos_version
}

output "shape" {
  description = "Cluster shape."
  value       = var.shape
}
