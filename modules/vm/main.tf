locals {
  recipe_id_valid = var.recipe_id != null && can(regex("(?i)^[0-9a-f]{40}$", var.recipe_id))
  recipe_id       = local.recipe_id_valid ? var.recipe_id : "0000000000000000000000000000000000000000"

  image_vm_id  = one(vergeio_vm.image[*].id)
  recipe_vm_id = one(vergeio_vm_recipe_instance.this[*].vm_id)
  vm_id = try(coalesce(
    local.image_vm_id,
    local.recipe_vm_id == null ? null : tostring(local.recipe_vm_id),
  ), null)
  vm_member = local.vm_id == null ? null : "vms/${local.vm_id}"

  # Fixed length. A conditional list is unknown while variables are unknown,
  # and vergeio_vm.cloudinit_files rejects an unknown list. The bodies change
  # on this same VM, so adding or clearing cloudinit_user_data does not
  # replace it. An omitted user-data body is an empty cloud-config.
  cloudinit_user_data = var.cloudinit_user_data == null ? "#cloud-config\n" : var.cloudinit_user_data
  cloudinit_meta_data = coalesce(var.cloudinit_meta_data, "instance-id: ${var.name}\nlocal-hostname: ${var.name}\n")
}

resource "vergeio_snapshot_profile" "this" {
  count = var.source_type == "image" && var.snapshot_profile != null ? 1 : 0

  name            = try(var.snapshot_profile.name, "unused")
  description     = try(var.snapshot_profile.description, null)
  ignore_warnings = try(var.snapshot_profile.ignore_warnings, null)

  dynamic "period" {
    for_each = try(var.snapshot_profile.periods, [])

    content {
      name          = period.value.name
      frequency     = period.value.frequency
      retention     = period.value.retention
      hour          = period.value.hour
      minute        = period.value.minute
      day_of_week   = period.value.day_of_week
      day_of_month  = period.value.day_of_month
      month         = period.value.month
      quiesce       = period.value.quiesce
      max_tier      = period.value.max_tier
      min_snapshots = period.value.min_snapshots
      immutable     = period.value.immutable
    }
  }
}

resource "vergeio_vm" "image" {
  count = var.source_type == "image" ? 1 : 0

  name                 = var.name
  description          = var.description
  enabled              = var.enabled
  os_family            = var.os_family
  cpu_cores            = var.cpu_cores
  ram                  = var.ram
  machine_type         = var.machine_type
  cluster              = var.cluster
  powerstate           = var.powerstate
  guest_agent          = var.guest_agent
  cloudinit_datasource = coalesce(var.cloudinit_datasource, "nocloud")
  snapshot_profile     = try(tonumber(vergeio_snapshot_profile.this[0].id), var.snapshot_profile_id)

  cloudinit_files = [
    {
      name     = "user-data"
      contents = local.cloudinit_user_data
    },
    {
      name     = "meta-data"
      contents = local.cloudinit_meta_data
    },
  ]

  boot_disk {
    name   = var.boot_disk_name
    size   = var.boot_disk_size
    media  = var.media_source_id == null ? "disk" : "import"
    source = var.media_source_id
  }
}

resource "vergeio_vm_recipe_instance" "this" {
  count = var.source_type == "recipe" ? 1 : 0

  name        = var.name
  recipe_id   = local.recipe_id
  answers     = var.recipe_answers
  auto_update = var.recipe_auto_update
}

resource "vergeio_vm_nic" "this" {
  for_each = var.source_type == "image" ? { for nic in var.nics : nic.name => nic } : {}

  vm_id            = local.vm_id
  name             = each.value.name
  description      = each.value.description
  interface        = each.value.interface
  vnet             = each.value.vnet
  assign_ipaddress = each.value.assign_ipaddress
  ipaddress        = each.value.ipaddress
}

resource "vergeio_tag_member" "this" {
  for_each = var.tag_ids

  tag_id = each.value
  member = local.vm_member
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition     = var.source_type != "image" || var.media_source_id != null || var.boot_disk_size != null
      error_message = "An image VM needs media_source_id or boot_disk_size."
    }

    precondition {
      condition     = var.source_type != "recipe" || local.recipe_id_valid
      error_message = "recipe_id is required when source_type is recipe and must be a 40-character hexadecimal recipe key."
    }

    precondition {
      condition     = var.source_type != "image" || var.recipe_id == null
      error_message = "recipe_id is used when source_type is recipe."
    }

    precondition {
      condition     = var.source_type != "recipe" || var.media_source_id == null
      error_message = "media_source_id is used when source_type is image."
    }

    precondition {
      condition     = var.source_type != "recipe" || (var.cloudinit_user_data == null && var.cloudinit_meta_data == null)
      error_message = "cloudinit_user_data is used for an image VM. Put recipe answers in recipe_answers. vergeio_vm_recipe_instance has no cloudinit_files attribute."
    }

    precondition {
      condition     = var.source_type != "recipe" || length(var.nics) == 0
      error_message = "nics is used for an image VM. A recipe network answer goes in recipe_answers."
    }

    precondition {
      condition     = var.source_type != "recipe" || (var.snapshot_profile == null && var.snapshot_profile_id == null)
      error_message = "snapshot_profile is used for an image VM. vergeio_vm_recipe_instance has no snapshot_profile attribute."
    }

    precondition {
      condition     = var.snapshot_profile == null || var.snapshot_profile_id == null
      error_message = "Set snapshot_profile or snapshot_profile_id."
    }

    precondition {
      condition     = var.cloudinit_datasource == null || contains(["nocloud", "config_drive_v2"], var.cloudinit_datasource)
      error_message = "cloudinit_datasource must be nocloud or config_drive_v2."
    }
  }
}
