output "id" {
  description = "VM key. For a recipe this is vergeio_vm_recipe_instance.vm_id."
  value       = local.vm_id
}

output "name" {
  description = "VM name."
  value       = var.name
}

output "source_type" {
  description = "image or recipe."
  value       = var.source_type
}

output "recipe_instance_id" {
  description = "Recipe instance key. Null for an image VM."
  value       = one(vergeio_vm_recipe_instance.this[*].id)
}

output "snapshot_profile_id" {
  description = "Snapshot profile key when this module creates one. Null when snapshot_profile is omitted or an existing snapshot_profile_id is attached."
  value       = one(vergeio_snapshot_profile.this[*].id)
}

output "guest_agent_ips" {
  description = "IP addresses reported by the guest agent on an image VM."
  value       = try(coalesce(one(vergeio_vm.image[*].guest_agent_ips), one(vergeio_vm.image_cloudinit[*].guest_agent_ips)), null)
}

output "nic_ids" {
  description = "NIC keys for an image VM, keyed by NIC name."
  value       = { for name, nic in vergeio_vm_nic.this : name => nic.id }
}

output "tag_ids" {
  description = "Keys of tags created by this module, keyed by category/name."
  value       = { for key, tag in vergeio_tag.this : key => tag.id }
}
