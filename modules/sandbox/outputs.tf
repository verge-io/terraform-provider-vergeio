output "name" {
  description = "Sandbox name."
  value       = var.name
}

output "network_id" {
  description = "Sandbox network key. Null for a tenant sandbox."
  value       = try(vergeio_network.sandbox[0].id, null)
}

output "tenant_id" {
  description = "Copied tenant key. Null for a VM sandbox."
  value       = try(vergeio_tenant_copy.this[0].id, null)
}

output "snapshot_id" {
  description = "Tenant snapshot key that was required to exist. The copy is the current tenant. Null for a VM sandbox."
  value       = try(vergeio_tenant_copy.this[0].snapshot_id, null)
}

output "vm_ids" {
  description = "Copied VM keys, keyed by the source VM key."
  value       = { for key, vm in vergeio_vm_clone.this : key => vm.id }
}

output "vm_snapshot_ids" {
  description = "machine_snapshots key cloned for each source VM key."
  value       = { for key, vm in vergeio_vm_clone.this : key => vm.snapshot_id }
}

output "isolated" {
  description = "True when the sandbox was not given an external route."
  value       = !var.allow_external
}
