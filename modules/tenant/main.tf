resource "vergeio_tenant" "this" {
  name                = var.name
  description         = var.description
  password_wo         = var.tenant_password_wo
  password_wo_version = var.tenant_password_wo_version
  change_password     = var.change_password
  powerstate          = var.powerstate
  isolate             = var.isolate
}

resource "vergeio_tenant_node" "this" {
  for_each = { for node in var.nodes : node.name => node }

  tenant_id      = vergeio_tenant.this.id
  name           = each.value.name
  description    = each.value.description
  cpu_cores      = each.value.cpu_cores
  ram            = each.value.ram
  enabled        = each.value.enabled
  on_power_loss  = each.value.on_power_loss
  cluster        = each.value.cluster
  preferred_node = each.value.preferred_node
}

resource "vergeio_tenant_storage" "this" {
  for_each = { for item in var.storage : tostring(item.tier) => item }

  tenant_id   = vergeio_tenant.this.id
  tier        = each.value.tier
  provisioned = each.value.provisioned
}

resource "vergeio_tenant_network_block" "this" {
  for_each = { for block in var.network_blocks : block.cidr => block }

  tenant_id             = vergeio_tenant.this.id
  network_id            = each.value.network_id
  cidr                  = each.value.cidr
  description           = each.value.description
  apply_parent_firewall = each.value.apply_parent_firewall
}

resource "vergeio_tenant_external_ip" "this" {
  for_each = { for addr in var.external_ips : addr.ip => addr }

  tenant_id             = vergeio_tenant.this.id
  network_id            = each.value.network_id
  ip                    = each.value.ip
  hostname              = each.value.hostname
  description           = each.value.description
  apply_parent_firewall = each.value.apply_parent_firewall
}

resource "vergeio_user" "admin" {
  name                = var.admin_user.name
  displayname         = var.admin_user.displayname
  email               = var.admin_user.email
  enabled             = var.admin_user.enabled
  type                = var.admin_user.type
  password_wo         = var.admin_password_wo
  password_wo_version = var.admin_password_wo_version
  change_password     = var.admin_user.change_password
}

resource "vergeio_member" "admin" {
  group  = var.admin_group_id
  member = format("users/%s", vergeio_user.admin.id)
}
