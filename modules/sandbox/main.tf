locals {
  use_vm_inventory     = var.vms != null
  use_tenant_inventory = var.tenant_snapshots != null

  tagged_vms = flatten([
    for item in data.vergeio_vms.tagged : [
      for vm in item.vms : vm if vm.is_snapshot != true
    ]
  ])

  inventory = local.use_vm_inventory ? var.vms : [
    for vm in local.tagged_vms : {
      key  = vm.key
      name = vm.name
      snapshots = [
        for snap in data.vergeio_vm_snapshots.source[tostring(vm.key)].snapshots : {
          id      = snap.id
          name    = snap.name
          created = snap.created
        }
      ]
    }
  ]

  vm_stamps = {
    for vm in local.inventory : tostring(vm.key) => sort([
      for snap in vm.snapshots : format("%020d/%020d", snap.created, tonumber(snap.id))
    ])
  }

  chosen_vm_snaps = {
    for vm in local.inventory : tostring(vm.key) => one([
      for snap in vm.snapshots : snap
      if var.snapshot_name == null ? format("%020d/%020d", snap.created, tonumber(snap.id)) == local.vm_stamps[tostring(vm.key)][length(local.vm_stamps[tostring(vm.key)]) - 1] : snap.name == var.snapshot_name
    ])
    if length(vm.snapshots) > 0 && (
      var.snapshot_name == null || length([for snap in vm.snapshots : snap if snap.name == var.snapshot_name]) == 1
    )
  }

  tenant_snaps = local.use_tenant_inventory ? var.tenant_snapshots : flatten([
    for item in data.vergeio_tenant_snapshots.source : [
      for snap in item.snapshots : {
        id      = snap.id
        name    = snap.name
        created = snap.created
      }
    ]
  ])
  tenant_stamps = sort([
    for snap in local.tenant_snaps : format("%020d/%020d", snap.created, tonumber(snap.id))
  ])
  tenant_newest = length(local.tenant_stamps) == 0 ? "" : local.tenant_stamps[length(local.tenant_stamps) - 1]
  chosen_tenant_snaps = var.source_type != "tenant" ? [] : [
    for snap in local.tenant_snaps : snap
    if var.snapshot_name == null ? format("%020d/%020d", snap.created, tonumber(snap.id)) == local.tenant_newest : snap.name == var.snapshot_name
  ]

  sandbox_cidr = coalesce(var.cidr, "192.0.2.0/24")
}

data "vergeio_tenant_snapshots" "source" {
  count = var.source_type == "tenant" && !local.use_tenant_inventory ? 1 : 0

  tenant_id = coalesce(var.tenant_id, "1")
}

data "vergeio_vms" "tagged" {
  count = var.source_type == "vm" && !local.use_vm_inventory && var.tag != null ? 1 : 0

  tag         = var.tag
  is_snapshot = false
}

data "vergeio_vm_snapshots" "source" {
  for_each = local.use_vm_inventory ? {} : {
    for vm in local.tagged_vms : tostring(vm.key) => vm
  }

  vm_id = each.key
}

resource "vergeio_network" "sandbox" {
  count = var.source_type == "vm" ? 1 : 0

  name                 = var.name
  description          = "Sandbox ${var.name}"
  enabled              = true
  type                 = "internal"
  network              = local.sandbox_cidr
  ipaddress            = cidrhost(local.sandbox_cidr, 1)
  dhcp_enabled         = false
  layer2_type          = "none"
  interface_vnet       = 0
  powerstate           = var.allow_external
  vnet_default_gateway = var.allow_external ? var.external_network_id : null
}

resource "vergeio_vm_clone" "this" {
  for_each = var.source_type == "vm" ? local.chosen_vm_snaps : {}

  snapshot_id = each.value.id
  name        = "${var.name}-${one([for vm in local.inventory : vm.name if tostring(vm.key) == each.key])}"
  network_id  = try(vergeio_network.sandbox[0].id, "1")
}

resource "vergeio_tenant_copy" "this" {
  count = var.source_type == "tenant" && length(local.chosen_tenant_snaps) == 1 ? 1 : 0

  tenant_id   = coalesce(var.tenant_id, "1")
  snapshot_id = try(local.chosen_tenant_snaps[0].id, "1")
  name        = var.name
  isolate     = !var.allow_external
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition     = var.source_type != "tenant" || (var.tenant_id != null && var.tag == null && var.vms == null && length(local.chosen_tenant_snaps) == 1)
      error_message = "A tenant sandbox needs tenant_id and exactly one snapshot. Omit snapshot_name to use the newest snapshot. tag and vms select VMs."
    }

    precondition {
      condition     = var.source_type != "vm" || (var.tenant_id == null && var.cidr != null && ((var.vms == null && var.tag != null) || (var.vms != null && var.tag == null && length(var.vms) > 0)) && length(local.chosen_vm_snaps) == length(local.inventory))
      error_message = "A VM sandbox needs cidr, either tag or vms, at least one VM, and one snapshot on each of those VMs."
    }

    precondition {
      condition     = !var.allow_external || var.source_type != "vm" || var.external_network_id != null
      error_message = "allow_external on a VM sandbox needs external_network_id."
    }

    precondition {
      condition     = var.allow_external || var.external_network_id == null
      error_message = "Set allow_external to true to use external_network_id."
    }
  }
}
