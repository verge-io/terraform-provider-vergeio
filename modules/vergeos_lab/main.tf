locals {
  letters = "abcdefghijklmnopqrstuvwxyz"

  controller_tier1 = var.shape == "uci" ? 0 : var.tier1_disk_count
  scale_count      = var.shape == "scale_out" ? var.scale_out_count : 0
  storage_count    = var.shape == "uci" ? var.storage_count : 0
  compute_count    = contains(["hybrid", "uci"], var.shape) ? var.compute_count : 0
  create_storage   = var.shape == "uci" ? "true" : "false"
  create_compute   = contains(["hybrid", "uci"], var.shape) ? "true" : "false"
  fabric1_prefix   = split("/", var.core_fabric_1_cidr)[1]
  fabric2_prefix   = split("/", var.core_fabric_2_cidr)[1]
  fabric1_router   = cidrhost(var.core_fabric_1_cidr, 254)
  fabric2_router   = cidrhost(var.core_fabric_2_cidr, 254)

  controllers = {
    "controller-1" = {
      role    = "controller"
      number  = 1
      tier0   = var.tier0_disk_count
      tier1   = local.controller_tier1
      cpu     = var.cpu_cores
      ram     = var.ram
      vm_name = "${var.name}-controller-1"
    }
    "controller-2" = {
      role    = "controller"
      number  = 2
      tier0   = var.tier0_disk_count
      tier1   = local.controller_tier1
      cpu     = var.cpu_cores
      ram     = var.ram
      vm_name = "${var.name}-controller-2"
    }
  }

  scale_nodes = {
    for i in range(local.scale_count) : "scale-${i + 1}" => {
      role    = "scale"
      number  = 3 + i
      tier0   = var.tier0_disk_count
      tier1   = var.tier1_disk_count
      cpu     = var.cpu_cores
      ram     = var.ram
      vm_name = "${var.name}-scale-${i + 1}"
    }
  }

  storage_nodes = {
    for i in range(local.storage_count) : "storage-${i + 1}" => {
      role    = "storage"
      number  = 3 + i
      tier0   = var.tier0_disk_count
      tier1   = var.storage_tier1_disk_count
      cpu     = var.storage_cpu_cores
      ram     = var.storage_ram
      vm_name = "${var.name}-storage-${i + 1}"
    }
  }

  compute_nodes = {
    for i in range(local.compute_count) : "compute-${i + 1}" => {
      role    = "compute"
      number  = 3 + local.storage_count + i
      tier0   = 0
      tier1   = 0
      cpu     = var.compute_cpu_cores
      ram     = var.compute_ram
      vm_name = "${var.name}-compute-${i + 1}"
    }
  }

  # Keys are controller-1, scale-1, storage-1, and compute-1. Counts are
  # numbers known at plan time, so these keys are known at plan time.
  nodes = merge(local.controllers, local.scale_nodes, local.storage_nodes, local.compute_nodes)

  disk_count = {
    for key, node in local.nodes : key => node.tier0 + node.tier1 + (node.role == "compute" ? 1 : 0)
  }

  drive_list = {
    for key, node in local.nodes : key => join(" ", [
      for i in range(local.disk_count[key]) : "/dev/sd${substr(local.letters, i, 1)}"
    ])
  }

  tier_list = {
    for key, node in local.nodes : key => join(" ", concat(
      [for i in range(node.tier0) : "0"],
      [for i in range(node.tier1) : "1"],
    ))
  }

  tier0_drives = merge([
    for key, node in local.nodes : {
      for i in range(node.tier0) : "${key}-tier0-${i}" => {
        node_key = key
        name     = "tier0-${i}"
        size     = var.tier0_disk_size_gb
        orderid  = i
      }
    }
  ]...)

  tier1_drives = merge([
    for key, node in local.nodes : {
      for i in range(node.tier1) : "${key}-tier1-${i}" => {
        node_key = key
        name     = "tier1-${i}"
        size     = node.role == "storage" ? var.storage_tier1_disk_size_gb : var.tier1_disk_size_gb
        orderid  = node.tier0 + i
      }
    }
  ]...)

  compute_drives = {
    for key, node in local.nodes : "${key}-disk" => {
      node_key = key
      name     = "disk"
      size     = var.compute_disk_size_gb
      orderid  = 0
    } if node.role == "compute"
  }

  cdrom_drives = {
    for key, node in local.nodes : key => node if var.media_source_id != null
  }

  # The password makes this map sensitive. It is not a for_each key.
  user_data = {
    for key, node in local.nodes : key => templatefile("${path.module}/templates/seed.tftpl", {
      vergeos_version  = var.vergeos_version
      role             = node.role
      number           = tostring(node.number)
      system_name      = var.name
      admin_username   = var.admin_username
      timezone         = var.timezone
      admin_password   = var.admin_password
      admin_email      = var.admin_email
      core_addr        = "100.96.0.${node.number + 1}/24"
      external_ip      = var.external_ip
      external_prefix  = tostring(var.external_prefix)
      external_gateway = var.external_gateway
      external_dns     = var.external_dns
      fabric1_addr     = "${cidrhost(var.core_fabric_1_cidr, node.number)}/${local.fabric1_prefix}"
      fabric2_addr     = "${cidrhost(var.core_fabric_2_cidr, node.number)}/${local.fabric2_prefix}"
      drive_list       = local.drive_list[key]
      tier_list        = local.tier_list[key]
      create_storage   = local.create_storage
      create_compute   = local.create_compute
    })
  }
}

resource "vergeio_network" "core_1" {
  name           = "${var.name}-core-1"
  description    = "${var.name}-core-1"
  enabled        = true
  type           = "internal"
  network        = var.core_fabric_1_cidr
  ipaddress      = local.fabric1_router
  ipaddress_type = "none"
  dhcp_enabled   = false
  mtu            = 9142
  powerstate     = var.network_powerstate
  on_power_loss  = var.network_powerstate ? "power_on" : "leave_off"
}

resource "vergeio_network" "core_2" {
  name           = "${var.name}-core-2"
  description    = "${var.name}-core-2"
  enabled        = true
  type           = "internal"
  network        = var.core_fabric_2_cidr
  ipaddress      = local.fabric2_router
  ipaddress_type = "none"
  dhcp_enabled   = false
  mtu            = 9142
  powerstate     = var.network_powerstate
  on_power_loss  = var.network_powerstate ? "power_on" : "leave_off"
}

resource "vergeio_vm" "node" {
  for_each = local.nodes

  name                  = each.value.vm_name
  description           = each.value.vm_name
  os_family             = "linux"
  os_description        = "VergeOS ${var.vergeos_version}"
  cpu_cores             = each.value.cpu
  ram                   = each.value.ram
  machine_type          = "q35"
  enabled               = true
  powerstate            = var.powerstate
  on_power_loss         = var.powerstate ? "power_on" : "leave_off"
  guest_agent           = true
  nested_virtualization = true
  disable_hypervisor    = true
  shutdown_on_destroy   = "kill"
  cloudinit_datasource  = "nocloud"
  cluster               = var.cluster_id
  boot_order            = var.media_source_id == null ? null : "dc"
  # A null timeout is left unset. The provider waits the full timeout even
  # when the guest never reports an address, including when the VM is off.
  wait_for_guest_ip_timeout = var.powerstate ? var.wait_for_guest_ip_timeout : null

  # Fixed length. An unknown list fails planning. Both bodies stay on this VM.
  cloudinit_files = [
    {
      name     = "user-data"
      contents = local.user_data[each.key]
    },
    {
      name     = "meta-data"
      contents = "instance-id: ${each.value.vm_name}\nlocal-hostname: ${each.value.vm_name}\n"
    },
  ]
}

resource "vergeio_vm_drive" "tier0" {
  for_each = local.tier0_drives

  vm_id       = vergeio_vm.node[each.value.node_key].id
  name        = each.value.name
  description = "${local.nodes[each.value.node_key].vm_name}-${each.value.name}"
  disksize    = each.value.size
  interface   = "virtio-scsi"
  orderid     = each.value.orderid
}

resource "vergeio_vm_drive" "tier1" {
  for_each = local.tier1_drives

  vm_id       = vergeio_vm.node[each.value.node_key].id
  name        = each.value.name
  description = "${local.nodes[each.value.node_key].vm_name}-${each.value.name}"
  disksize    = each.value.size
  interface   = "virtio-scsi"
  orderid     = each.value.orderid
}

resource "vergeio_vm_drive" "compute" {
  for_each = local.compute_drives

  vm_id       = vergeio_vm.node[each.value.node_key].id
  name        = each.value.name
  description = "${local.nodes[each.value.node_key].vm_name}-${each.value.name}"
  disksize    = each.value.size
  interface   = "virtio-scsi"
  orderid     = each.value.orderid
}

resource "vergeio_vm_drive" "cdrom" {
  for_each = local.cdrom_drives

  vm_id        = vergeio_vm.node[each.key].id
  name         = "cdrom"
  description  = "${each.value.vm_name}-cdrom"
  media        = "cdrom"
  media_source = var.media_source_id
  interface    = "ahci"
  orderid      = 20
}

resource "vergeio_vm_nic" "external" {
  for_each = local.nodes

  vm_id            = vergeio_vm.node[each.key].id
  name             = "external"
  description      = "${each.value.vm_name}-external"
  interface        = "virtio"
  vnet             = var.external_network_id
  assign_ipaddress = false
}

resource "vergeio_vm_nic" "core_1" {
  for_each = local.nodes

  vm_id            = vergeio_vm.node[each.key].id
  name             = "core-1"
  description      = "${each.value.vm_name}-core-1"
  interface        = "virtio"
  vnet             = tonumber(vergeio_network.core_1.id)
  assign_ipaddress = false
}

resource "vergeio_vm_nic" "core_2" {
  for_each = local.nodes

  vm_id            = vergeio_vm.node[each.key].id
  name             = "core-2"
  description      = "${each.value.vm_name}-core-2"
  interface        = "virtio"
  vnet             = tonumber(vergeio_network.core_2.id)
  assign_ipaddress = false
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition     = var.core_fabric_1_cidr != var.core_fabric_2_cidr
      error_message = "core_fabric_1_cidr and core_fabric_2_cidr must be different networks."
    }

    precondition {
      condition     = var.wait_for_guest_ip_timeout == null || var.powerstate
      error_message = "wait_for_guest_ip_timeout waits even when the VM is off. Set powerstate true or leave the timeout unset."
    }

    precondition {
      condition     = max([for node in local.nodes : node.number]...) < 254
      error_message = "A node number must stay below 254 so it does not use the fabric router address."
    }

    precondition {
      condition     = var.tier0_disk_count + var.tier1_disk_count <= 26 && var.tier0_disk_count + var.storage_tier1_disk_count <= 26
      error_message = "A node can have at most 26 virtio-scsi disks."
    }
  }
}
