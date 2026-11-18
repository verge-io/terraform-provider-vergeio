output "system_key" {
  description = "Inventory key of the system this state configures."
  value       = var.system_key
}

output "waves" {
  description = "Waves included in this configuration."
  value       = var.waves
}

output "pending_waves" {
  description = "Waves named by the inventory and not included in this configuration."
  value       = setsubtract(toset([for _, tenant in var.tenants : tenant.wave]), var.waves)
}

output "tenant_keys" {
  description = "Inventory keys of the tenants this configuration creates."
  value       = toset(keys(module.tenant))
}

output "pending_tenant_keys" {
  description = "Inventory keys left out because their wave is not included."
  value       = toset([for key, tenant in var.tenants : key if !contains(var.waves, tenant.wave)])
}

output "tenant_ids" {
  description = "Tenant keys assigned by VergeOS, keyed by inventory key."
  value       = { for key, tenant in module.tenant : key => tenant.id }
}

output "tenant_names" {
  description = "Tenant names, keyed by inventory key."
  value       = { for key, tenant in module.tenant : key => tenant.name }
}

output "tenant_ui_addresses" {
  description = "Tenant UI addresses, keyed by inventory key. Empty until VergeOS assigns the first external IP. A second configuration uses this as the provider host for the inside of the tenant."
  value       = { for key, tenant in module.tenant : key => tenant.ui_address }
}

output "admin_user_ids" {
  description = "Parent-system admin user keys, keyed by tenant inventory key."
  value       = { for key, tenant in module.tenant : key => tenant.admin_user_id }
}

output "network_ids" {
  description = "Internal network keys, keyed by tenant inventory key and network policy key."
  value       = { for key, network in module.network : key => network.id }
}

output "network_names" {
  description = "Internal network names, keyed by tenant inventory key and network policy key."
  value       = { for key, network in module.network : key => network.name }
}

output "network_cidrs" {
  description = "Internal network CIDRs, keyed by tenant inventory key and network policy key."
  value       = { for key, network in module.network : key => network.cidr }
}

output "network_addresses" {
  description = "Bare router addresses, keyed by tenant inventory key and network policy key."
  value       = { for key, network in module.network : key => network.ipaddress }
}

output "dns_zone_ids" {
  description = "DNS zone keys, keyed by tenant inventory key and network policy key. Null when that tenant network sets no dns."
  value       = { for key, network in module.network : key => network.dns_zone_id }
}

output "firewall_ids" {
  description = "Firewall policy ids, keyed by tenant inventory key and network policy key. The id is the network key."
  value       = { for key, network in module.network : key => network.firewall_id }
}

output "firewall_rule_names" {
  description = "Firewall rule names applied to each selected tenant network, keyed by tenant inventory key and network policy key."
  value = {
    for key, binding in local.network_bindings :
    key => [for rule in var.networks[binding.network_key].firewall_rules : rule.name]
  }
}

output "tenant_admin_group_id" {
  description = "Key of the tenant admin group this module creates."
  value       = vergeio_group.tenant_admins.id
}

output "tenant_admin_group_name" {
  description = "Name of the tenant admin group this module creates."
  value       = vergeio_group.tenant_admins.name
}

output "baseline_profile" {
  description = "Baseline profile passed to modules/baseline."
  value       = module.baseline.profile
}

output "baseline_controls" {
  description = "Baseline control mapping. applied is true when this configuration writes that control."
  value       = module.baseline.controls
}

output "baseline_setting_ids" {
  description = "vergeio_setting ids written for the baseline, keyed by control id. Empty when baseline_apply_settings is false."
  value       = module.baseline.setting_ids
}

output "baseline_setting_keys" {
  description = "VergeOS setting keys written for the baseline, keyed by control id."
  value       = module.baseline.setting_keys
}

output "baseline_user_ids" {
  description = "Baseline user keys, keyed by user name."
  value       = module.baseline.user_ids
}

output "baseline_group_ids" {
  description = "Baseline group keys, keyed by group name."
  value       = module.baseline.group_ids
}

output "baseline_certificate_ids" {
  description = "Baseline certificate keys, keyed by certificate name."
  value       = module.baseline.certificate_ids
}

output "baseline_api_key_ids" {
  description = "Baseline API key ids, keyed by user name and key name. The bearer token is not stored."
  value       = module.baseline.api_key_ids
}
