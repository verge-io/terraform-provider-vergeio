output "id" {
  description = "Tenant key assigned by VergeOS."
  value       = vergeio_tenant.this.id
}

output "name" {
  description = "Tenant name."
  value       = vergeio_tenant.this.name
}

output "ui_address" {
  description = "IP address of the tenant UI. Empty until VergeOS assigns the first external IP."
  value       = vergeio_tenant.this.ui_address
}

output "uuid" {
  description = "Tenant UUID assigned by VergeOS."
  value       = vergeio_tenant.this.uuid
}

output "vnet" {
  description = "Key of the network VergeOS creates for the tenant."
  value       = vergeio_tenant.this.vnet
}

output "node_ids" {
  description = "Tenant node keys, keyed by node name."
  value       = { for name, node in vergeio_tenant_node.this : name => node.id }
}

output "storage_ids" {
  description = "Tenant storage keys, keyed by tier."
  value       = { for tier, item in vergeio_tenant_storage.this : tier => item.id }
}

output "network_block_ids" {
  description = "Routed block keys, keyed by CIDR."
  value       = { for cidr, block in vergeio_tenant_network_block.this : cidr => block.id }
}

output "network_block_parent_firewall_pending" {
  description = "True when the parent network still needs its firewall rules applied for that block."
  value       = { for cidr, block in vergeio_tenant_network_block.this : cidr => block.parent_firewall_pending }
}

output "external_ip_ids" {
  description = "External IP keys, keyed by address. The first assigned IP is the tenant UI address."
  value       = { for ip, addr in vergeio_tenant_external_ip.this : ip => addr.id }
}

output "external_ip_parent_firewall_pending" {
  description = "True when the parent network still needs its firewall rules applied for that address."
  value       = { for ip, addr in vergeio_tenant_external_ip.this : ip => addr.parent_firewall_pending }
}

output "admin_user_id" {
  description = "Key of the parent-system admin user."
  value       = vergeio_user.admin.id
}

output "admin_user_name" {
  description = "Name of the parent-system admin user."
  value       = vergeio_user.admin.name
}
