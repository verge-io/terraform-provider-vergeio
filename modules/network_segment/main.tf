locals {
  dns_records = {
    for record in try(var.dns.records, []) :
    "${coalesce(record.host, "@")}/${record.type}/${record.value}" => record
  }
}

resource "vergeio_network" "this" {
  name                 = var.name
  description          = var.description
  enabled              = var.enabled
  type                 = "internal"
  network              = var.cidr
  ipaddress            = var.ipaddress
  vnet_default_gateway = var.external_network_id
  dhcp_enabled         = var.dhcp_enabled
  dynamic_dhcp         = var.dhcp_enabled && var.dynamic_dhcp
  dhcp_sequential      = var.dhcp_enabled && var.dynamic_dhcp && var.dhcp_sequential
  dhcp_start           = var.dhcp_enabled ? var.dhcp_start : null
  dhcp_stop            = var.dhcp_enabled ? var.dhcp_stop : null
  dnslist              = length(var.dns_servers) == 0 ? null : join(",", var.dns_servers)
  domain               = var.domain
  powerstate           = var.powerstate
}

resource "vergeio_network_rules" "this" {
  vnet = vergeio_network.this.id

  rule = [
    for rule in var.firewall_rules : {
      name              = rule.name
      description       = rule.description == null ? "" : rule.description
      direction         = coalesce(rule.direction, "incoming")
      action            = coalesce(rule.action, "accept")
      protocol          = coalesce(rule.protocol, "any")
      source_ip         = rule.source_ip == null ? "" : rule.source_ip
      source_ports      = rule.source_ports == null ? "" : rule.source_ports
      destination_ip    = rule.destination_ip == null ? "" : rule.destination_ip
      destination_ports = rule.destination_ports == null ? "" : rule.destination_ports
      target_ip         = rule.target_ip == null ? "" : rule.target_ip
      target_ports      = rule.target_ports == null ? "" : rule.target_ports
      enabled           = coalesce(rule.enabled, true)
    }
  ]
}

resource "vergeio_network_dns_view" "this" {
  count = var.dns == null ? 0 : 1

  network_id    = vergeio_network.this.id
  name          = try(var.dns.view_name, "internal")
  match_clients = try(coalesce(var.dns.match_clients, "${var.cidr};"), null)
  recursion     = try(var.dns.recursion, null)
  apply         = false
}

resource "vergeio_network_dns_zone" "this" {
  count = var.dns == null ? 0 : 1

  view_id = coalesce(one(vergeio_network_dns_view.this[*].id), "0")
  domain  = coalesce(try(var.dns.zone_domain, null), "unused.invalid")
  type    = "master"
  email   = try(var.dns.email, null)
  apply   = length(local.dns_records) == 0
}

resource "vergeio_network_dns_record" "this" {
  for_each = local.dns_records

  zone_id       = one(vergeio_network_dns_zone.this[*].id)
  host          = each.value.host
  type          = each.value.type
  value         = each.value.value
  ttl           = each.value.ttl
  description   = each.value.description
  mx_preference = each.value.mx_preference
  port          = each.value.port
  weight        = each.value.weight

  depends_on = [
    vergeio_network_dns_view.this,
    vergeio_network_dns_zone.this,
  ]
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition     = !var.dhcp_enabled || (var.dhcp_start != null && var.dhcp_stop != null)
      error_message = "dhcp_start and dhcp_stop are required when dhcp_enabled is true."
    }
  }
}
