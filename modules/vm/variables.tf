variable "name" {
  type        = string
  description = "VM name."

  validation {
    condition     = length(var.name) > 0
    error_message = "name must not be empty."
  }
}

variable "description" {
  type        = string
  description = "VM description."
  default     = null
}

variable "source_type" {
  type        = string
  description = "image creates a vergeio_vm from a media image. recipe creates a vergeio_vm_recipe_instance."

  validation {
    condition     = contains(["image", "recipe"], var.source_type)
    error_message = "source_type must be image or recipe."
  }
}

variable "media_source_id" {
  type        = number
  description = "Media image key for an image VM. vergeio_mediasources.mediasources[].id. Written to boot_disk.source with media import. Omit it and set boot_disk_size to create a blank disk."
  default     = null
}

variable "boot_disk_name" {
  type        = string
  description = "Boot disk name for an image VM."
  default     = "os"

  validation {
    condition     = length(var.boot_disk_name) > 0
    error_message = "boot_disk_name must not be empty."
  }
}

variable "boot_disk_size" {
  type        = number
  description = "Boot disk size in GB for an image VM. Omit to leave the size unset."
  default     = null

  validation {
    condition     = var.boot_disk_size == null || var.boot_disk_size > 0
    error_message = "boot_disk_size must be greater than 0."
  }
}

variable "recipe_id" {
  type        = string
  description = "Recipe key for a recipe VM. 40 hexadecimal characters. vergeio_vm_recipes.recipes[].id."
  default     = null
}

variable "recipe_answers" {
  type        = map(string)
  description = "Recipe question names and string answers. A disksize answer is bytes. A bool answer is true, false, yes, no, on, off, 1, or 0. A network answer is a network name, a vnet key, or __new_internal__. The provider marks this map sensitive because it can hold a guest password."
  default     = {}
}

variable "recipe_auto_update" {
  type        = bool
  description = "Update this VM when the recipe is updated. Omit to leave the platform default. Changing it replaces the VM."
  default     = null
}

variable "os_family" {
  type        = string
  description = "Guest OS family for an image VM."
  default     = "linux"

  validation {
    condition     = contains(["linux", "windows", "freebsd", "other"], var.os_family)
    error_message = "os_family must be linux, windows, freebsd, or other."
  }
}

variable "cpu_cores" {
  type        = number
  description = "CPU cores for an image VM."
  default     = 2

  validation {
    condition     = var.cpu_cores >= 1 && var.cpu_cores == floor(var.cpu_cores)
    error_message = "cpu_cores must be a whole number of at least 1."
  }
}

variable "ram" {
  type        = number
  description = "RAM for an image VM, in megabytes."
  default     = 2048

  validation {
    condition     = var.ram >= 1 && var.ram == floor(var.ram)
    error_message = "ram must be a whole number of at least 1."
  }
}

variable "machine_type" {
  type        = string
  description = "Machine type for an image VM, such as q35 or pc. Omit to leave the provider default."
  default     = null
}

variable "cluster" {
  type        = number
  description = "Compute cluster key for an image VM. Omit to leave the system default."
  default     = null
}

variable "enabled" {
  type        = bool
  description = "Whether an image VM is enabled."
  default     = true
}

variable "powerstate" {
  type        = bool
  description = "Whether an image VM is powered on. False leaves a new VM stopped."
  default     = false
}

variable "guest_agent" {
  type        = bool
  description = "Enable the guest agent on an image VM. Omit to leave the provider default."
  default     = null
}

variable "cloudinit_datasource" {
  type        = string
  description = "Cloud-init datasource for an image VM: nocloud or config_drive_v2. Image VMs always write cloud-init files, so this defaults to nocloud."
  default     = null
}

variable "cloudinit_user_data" {
  type        = string
  description = "user-data cloud-init body for an image VM. Stored in state. Null writes an empty cloud-config on the same VM. Setting or clearing it updates that file in place."
  default     = null

  validation {
    condition     = var.cloudinit_user_data == null || length(var.cloudinit_user_data) > 0
    error_message = "cloudinit_user_data must not be empty."
  }
}

variable "cloudinit_meta_data" {
  type        = string
  description = "meta-data cloud-init body for an image VM. Stored in state. Null writes instance-id and local-hostname from the VM name when cloudinit_user_data is set."
  default     = null

  validation {
    condition     = var.cloudinit_meta_data == null || length(var.cloudinit_meta_data) > 0
    error_message = "cloudinit_meta_data must not be empty."
  }
}

variable "nics" {
  type = list(object({
    name             = string
    vnet             = number
    description      = optional(string)
    interface        = optional(string)
    assign_ipaddress = optional(bool)
    ipaddress        = optional(string)
  }))
  description = "NICs for an image VM. vnet is the network key as a number, the same value as vergeio_vm_nic.vnet."
  default     = []

  validation {
    condition     = length(var.nics) == length(distinct([for nic in var.nics : nic.name]))
    error_message = "NIC names must be unique on the VM."
  }

  validation {
    condition = alltrue([
      for nic in var.nics :
      length(nic.name) > 0 && nic.vnet == floor(nic.vnet) &&
      (nic.interface == null || contains(["virtio", "e1000", "rtl8139", "pcnet", "e1000e", "igb", "vmxnet3"], nic.interface))
    ])
    error_message = "Each NIC needs a name and a whole vnet key. interface must be virtio, e1000, rtl8139, pcnet, e1000e, igb, or vmxnet3."
  }
}

variable "tag_ids" {
  type        = map(number)
  description = "Tag keys to assign to this VM. Each key is a name you choose and must be known at plan time. Each value is an existing vergeio_tag key and may be unknown until apply."
  default     = {}

  validation {
    condition     = length(var.tag_ids) == length(distinct([for id in values(var.tag_ids) : tostring(id)]))
    error_message = "tag_ids values must be unique."
  }
}

variable "snapshot_profile" {
  type = object({
    name            = string
    description     = optional(string)
    ignore_warnings = optional(bool)
    periods = list(object({
      name          = string
      frequency     = string
      retention     = number
      hour          = optional(number)
      minute        = optional(number)
      day_of_week   = optional(string)
      day_of_month  = optional(number)
      month         = optional(number)
      quiesce       = optional(bool)
      max_tier      = optional(string)
      min_snapshots = optional(number)
      immutable     = optional(bool)
    }))
  })
  description = "Snapshot profile to create and attach to an image VM. retention is seconds and is required on every period. Attach the key with vergeio_vm.snapshot_profile."
  default     = null

  validation {
    condition     = var.snapshot_profile == null || (length(var.snapshot_profile.name) > 0 && length(var.snapshot_profile.periods) > 0)
    error_message = "snapshot_profile needs a name and at least one period."
  }

  validation {
    condition = var.snapshot_profile == null || alltrue([
      for period in var.snapshot_profile.periods :
      length(period.name) > 0 &&
      contains(["custom", "hourly", "daily", "weekly", "monthly", "yearly"], period.frequency) &&
      period.retention >= 1 && period.retention == floor(period.retention) &&
      (period.hour == null || (period.hour >= 0 && period.hour <= 23 && period.hour == floor(period.hour))) &&
      (period.minute == null || (period.minute >= 0 && period.minute <= 59 && period.minute == floor(period.minute))) &&
      (period.day_of_week == null || contains(["sun", "mon", "tue", "wed", "thu", "fri", "sat", "any"], period.day_of_week)) &&
      (period.day_of_month == null || (period.day_of_month >= 0 && period.day_of_month <= 31 && period.day_of_month == floor(period.day_of_month))) &&
      (period.month == null || (period.month >= 0 && period.month <= 12 && period.month == floor(period.month))) &&
      (period.max_tier == null || contains(["1", "2", "3", "4", "5"], period.max_tier)) &&
      (period.min_snapshots == null || (period.min_snapshots >= 0 && period.min_snapshots == floor(period.min_snapshots)))
    ])
    error_message = "Each snapshot period needs a name, a frequency of custom, hourly, daily, weekly, monthly, or yearly, and a retention of at least 1 second. hour is 0-23, minute is 0-59, day_of_week is sun through sat or any, and max_tier is 1 through 5."
  }
}

variable "snapshot_profile_id" {
  type        = number
  description = "Existing snapshot profile key to attach to an image VM. Omit when snapshot_profile creates one."
  default     = null

  validation {
    condition     = var.snapshot_profile_id == null || var.snapshot_profile_id == floor(var.snapshot_profile_id)
    error_message = "snapshot_profile_id must be a whole number."
  }
}
