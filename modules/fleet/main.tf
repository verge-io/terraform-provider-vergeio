locals {
  selected_tenants = {
    for key, tenant in var.tenants : key => tenant
    if contains(var.waves, tenant.wave)
  }

  # Keys are the tenant inventory key and the network policy key. Both are
  # known at plan time. A tenant outside var.waves is not in this map.
  network_bindings = {
    for pair in flatten([
      for tenant_key, tenant in local.selected_tenants : [
        for network_key, assignment in tenant.networks : {
          tenant_key  = tenant_key
          network_key = network_key
        }
        if contains(keys(var.networks), network_key)
      ]
    ]) : "${pair.tenant_key}/${pair.network_key}" => pair
  }

  tenant_admin_permissions = {
    for perm in var.tenant_admin_group.permissions :
    "${perm.table}/${perm.object_id == null ? "table" : tostring(perm.object_id)}" => perm
  }

  inventory_cidrs = flatten([
    for tenant_key, tenant in var.tenants : [
      for network_key, assignment in tenant.networks : assignment.cidr
      if contains(keys(var.networks), network_key)
    ]
  ])

  # The for_each address is tenant key / network key. The VergeOS name joins
  # the tenant name and the network key with a hyphen, and both may contain
  # hyphens, so the name is checked on its own.
  network_names = flatten([
    for _, tenant in var.tenants : [
      for network_key, _ in tenant.networks : "${tenant.name}-${network_key}"
      if contains(keys(var.networks), network_key)
    ]
  ])
}

resource "vergeio_group" "tenant_admins" {
  name        = var.tenant_admin_group.name
  description = var.tenant_admin_group.description
  enabled     = var.tenant_admin_group.enabled
}

resource "vergeio_permission" "tenant_admins" {
  for_each = local.tenant_admin_permissions

  group_id  = vergeio_group.tenant_admins.id
  table     = each.value.table
  object_id = each.value.object_id
  list      = each.value.list
  read      = each.value.read
  create    = each.value.create
  modify    = each.value.modify
  delete    = each.value.delete
}

module "baseline" {
  source = "../baseline"

  profile                      = var.baseline_profile
  apply_settings               = var.baseline_apply_settings
  settings                     = var.baseline_settings
  api_key_max_lifetime_seconds = var.baseline_api_key_max_lifetime_seconds
  certificates                 = var.baseline_certificates
  certificate_private_keys     = var.baseline_certificate_private_keys
  certificate_eab_hmac_keys    = var.baseline_certificate_eab_hmac_keys
  groups                       = var.baseline_groups
  users                        = var.baseline_users
  passwords                    = var.baseline_passwords
  password_versions            = var.baseline_password_versions
}

module "tenant" {
  for_each = local.selected_tenants
  source   = "../tenant"

  name                       = each.value.name
  description                = each.value.description
  tenant_password_wo         = var.tenant_passwords[each.key]
  tenant_password_wo_version = lookup(var.tenant_password_versions, each.key, 1)
  change_password            = each.value.change_password
  powerstate                 = each.value.powerstate
  isolate                    = each.value.isolate
  nodes                      = each.value.nodes
  storage                    = each.value.storage
  network_blocks             = each.value.network_blocks
  external_ips               = each.value.external_ips
  admin_group_id             = tonumber(vergeio_group.tenant_admins.id)
  admin_password_wo          = var.admin_passwords[each.key]
  admin_password_wo_version  = lookup(var.admin_password_versions, each.key, 1)
  admin_user                 = each.value.admin_user
}

module "network" {
  for_each = local.network_bindings
  source   = "../network_segment"

  name                = "${var.tenants[each.value.tenant_key].name}-${each.value.network_key}"
  description         = var.networks[each.value.network_key].description
  cidr                = var.tenants[each.value.tenant_key].networks[each.value.network_key].cidr
  ipaddress           = var.tenants[each.value.tenant_key].networks[each.value.network_key].ipaddress
  enabled             = var.networks[each.value.network_key].enabled
  powerstate          = var.networks[each.value.network_key].powerstate
  dhcp_enabled        = var.networks[each.value.network_key].dhcp_enabled
  dynamic_dhcp        = var.networks[each.value.network_key].dynamic_dhcp
  dhcp_sequential     = var.networks[each.value.network_key].dhcp_sequential
  dhcp_start          = var.tenants[each.value.tenant_key].networks[each.value.network_key].dhcp_start
  dhcp_stop           = var.tenants[each.value.tenant_key].networks[each.value.network_key].dhcp_stop
  dns_servers         = var.networks[each.value.network_key].dns_servers
  domain              = var.tenants[each.value.tenant_key].networks[each.value.network_key].domain
  dns                 = var.tenants[each.value.tenant_key].networks[each.value.network_key].dns
  external_network_id = var.networks[each.value.network_key].external_network_id
  firewall_rules      = var.networks[each.value.network_key].firewall_rules
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition = alltrue([
        for wave in var.waves : contains([for _, tenant in var.tenants : tenant.wave], wave)
      ])
      error_message = "Every wave in waves must be the wave of at least one tenant. A wave that matches nothing is left out of the inventory."
    }

    precondition {
      condition     = toset(nonsensitive(keys(var.tenant_passwords))) == toset(keys(var.tenants)) && toset(nonsensitive(keys(var.admin_passwords))) == toset(keys(var.tenants))
      error_message = "tenant_passwords and admin_passwords must each contain every tenant inventory key, including tenants in a later wave."
    }

    precondition {
      condition = alltrue([
        for key, _ in var.tenant_password_versions : contains(keys(var.tenants), key)
        ]) && alltrue([
        for key, _ in var.admin_password_versions : contains(keys(var.tenants), key)
      ])
      error_message = "Password version keys must be tenant inventory keys."
    }

    precondition {
      condition = alltrue([
        for _, tenant in var.tenants : toset(keys(tenant.networks)) == toset(keys(var.networks))
      ])
      error_message = "Each tenant, including tenants in a later wave, needs a networks entry for every key in networks."
    }

    precondition {
      condition     = length(local.inventory_cidrs) == length(distinct(local.inventory_cidrs))
      error_message = "Tenant network CIDRs must be unique across the inventory. Two tenants cannot share a CIDR on this system."
    }

    precondition {
      condition     = length(local.network_names) == length(distinct(local.network_names))
      error_message = "Each VergeOS network name is the tenant name, a hyphen, and the network key. Those names must be unique. A tenant named west-net with network key app and a tenant named west with network key net-app both produce west-net-app."
    }

    precondition {
      condition = alltrue(flatten([
        for _, tenant in var.tenants : [
          for network_key, assignment in tenant.networks :
          !try(var.networks[network_key].dhcp_enabled, false) || (
            assignment.dhcp_start != null && assignment.dhcp_stop != null &&
            try(cidrhost("${assignment.dhcp_start}/${split("/", assignment.cidr)[1]}", 0) == cidrhost(assignment.cidr, 0), false) &&
            try(cidrhost("${assignment.dhcp_stop}/${split("/", assignment.cidr)[1]}", 0) == cidrhost(assignment.cidr, 0), false) &&
            assignment.dhcp_start != coalesce(assignment.ipaddress, try(cidrhost(assignment.cidr, 1), "")) &&
            assignment.dhcp_stop != coalesce(assignment.ipaddress, try(cidrhost(assignment.cidr, 1), ""))
          )
        ]
      ]))
      error_message = "When a network enables DHCP, every tenant needs dhcp_start and dhcp_stop inside that tenant CIDR. Neither address can be the router address."
    }

    precondition {
      condition = alltrue(flatten([
        for _, tenant in var.tenants : [
          for _, assignment in tenant.networks :
          assignment.ipaddress == null || try(cidrhost("${assignment.ipaddress}/${split("/", assignment.cidr)[1]}", 0) == cidrhost(assignment.cidr, 0), false)
        ]
      ]))
      error_message = "A tenant network ipaddress must be inside that tenant CIDR. Omit it to use the first host of the CIDR. The value is a bare IP."
    }

    precondition {
      condition = length(setintersection(
        toset([for _, tenant in var.tenants : tenant.admin_user.name]),
        toset([for user in var.baseline_users : user.name]),
      )) == 0
      error_message = "A tenant admin user and a baseline user cannot share a name."
    }

    precondition {
      condition     = !contains([for group in var.baseline_groups : group.name], var.tenant_admin_group.name)
      error_message = "tenant_admin_group.name is the group this module creates. It cannot match a baseline group name."
    }
  }
}
