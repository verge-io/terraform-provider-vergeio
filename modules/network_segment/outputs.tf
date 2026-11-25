output "id" {
  description = "Network key. Same value as vergeio_network.id."
  value       = vergeio_network.this.id
}

output "name" {
  description = "Network name."
  value       = vergeio_network.this.name
}

output "cidr" {
  description = "CIDR of this network."
  value       = vergeio_network.this.network
}

output "ipaddress" {
  description = "Address of this network."
  value       = vergeio_network.this.ipaddress
}

output "external_network_id" {
  description = "External network key written to vnet_default_gateway. Null when no outbound gateway is set."
  value       = vergeio_network.this.vnet_default_gateway
}

output "dns_view_id" {
  description = "DNS view key. Null when dns is omitted."
  value       = one(vergeio_network_dns_view.this[*].id)
}

output "dns_zone_id" {
  description = "DNS zone key. Null when dns is omitted."
  value       = one(vergeio_network_dns_zone.this[*].id)
}

output "firewall_id" {
  description = "Firewall policy id. Same value as the network key."
  value       = vergeio_network_rules.this.id
}
