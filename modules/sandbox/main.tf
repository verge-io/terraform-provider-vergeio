locals {
  use_vm_inventory = var.vms != null

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

  sandbox_cidr = coalesce(var.cidr, "192.0.2.0/24")

  # The network exists only when every selected VM has a snapshot to clone.
  # An empty selection plans no network, and a later empty selection removes
  # one that was already created. That removal is not a failed check.
  vm_selection_ready = (
    var.source_type == "vm" &&
    var.tenant_id == null &&
    var.cidr != null &&
    ((var.vms == null && var.tag != null) || (var.vms != null && var.tag == null)) &&
    length(local.inventory) > 0 &&
    length(local.chosen_vm_snaps) == length(local.inventory)
  )
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
  count = local.vm_selection_ready ? 1 : 0

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
  for_each = local.vm_selection_ready ? local.chosen_vm_snaps : {}

  snapshot_id = each.value.id
  name        = "${var.name}-${one([for vm in local.inventory : vm.name if tostring(vm.key) == each.key])}"
  network_id  = try(vergeio_network.sandbox[0].id, "1")
}

resource "vergeio_tenant_copy" "this" {
  count = var.source_type == "tenant" ? 1 : 0

  tenant_id = coalesce(var.tenant_id, "1")
  name      = var.name
  isolate   = !var.allow_external
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition     = var.source_type != "tenant" || (var.tenant_id != null && var.tag == null && var.vms == null && var.snapshot_name == null)
      error_message = "A tenant sandbox needs tenant_id. It clones the tenant's current state. snapshot_name, tag, and vms are for a VM sandbox."
    }

    precondition {
      condition     = var.source_type != "vm" || (var.tenant_id == null && var.cidr != null && ((var.vms == null && var.tag != null) || (var.vms != null && var.tag == null)) && (length(local.inventory) == 0 || length(local.chosen_vm_snaps) == length(local.inventory)))
      error_message = "A VM sandbox needs cidr and either tag or vms. Each selected VM needs one snapshot. An empty selection creates no network and does not block removal of a sandbox that already exists."
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
