variable "name" {
  type        = string
  description = "Sandbox name. The tenant copy uses it as its name. The VM network uses it as its name. Each copied VM is named name-vmname."

  validation {
    condition     = length(var.name) > 0
    error_message = "name must not be empty."
  }
}

variable "source_type" {
  type        = string
  description = "tenant clones the tenant's current state. vm clones every VM that has the tag from a snapshot."

  validation {
    condition     = contains(["tenant", "vm"], var.source_type)
    error_message = "source_type must be tenant or vm."
  }
}

variable "tenant_id" {
  type        = string
  description = "Source tenant key, vergeio_tenant.id. Required when source_type is tenant. The copy is the tenant's current state, not a snapshot. The tenant is only read."
  default     = null

  validation {
    condition     = var.tenant_id == null || can(regex("^[1-9][0-9]*$", var.tenant_id))
    error_message = "tenant_id must be a positive integer."
  }
}

variable "tag" {
  type        = string
  description = "Tag name or category/name. Required when source_type is vm. VMs assigned this tag are the source. The tag and the VMs are only read."
  default     = null

  validation {
    condition     = var.tag == null || length(var.tag) > 0
    error_message = "tag must not be empty."
  }
}

variable "snapshot_name" {
  type        = string
  description = "VM snapshot name to clone. Omit it to use the newest snapshot, the one with the greatest created time and then the greatest key. The snapshot is only read. Leave this unset for a tenant sandbox. A tenant sandbox clones the tenant's current state."
  default     = null

  validation {
    condition     = var.snapshot_name == null || length(var.snapshot_name) > 0
    error_message = "snapshot_name must not be empty."
  }
}

variable "cidr" {
  type        = string
  description = "CIDR of the sandbox network. Required when source_type is vm. The address is the network address, such as 192.168.91.0/24."
  default     = null

  validation {
    condition     = var.cidr == null || (can(cidrhost(var.cidr, 0)) && try(cidrhost(var.cidr, 0) == split("/", var.cidr)[0], false))
    error_message = "cidr must be a network address in CIDR notation, such as 192.168.91.0/24."
  }
}

variable "allow_external" {
  type        = bool
  description = "Give the sandbox a route outside itself. Default false. A VM sandbox then has no vnet_default_gateway, layer2_type none, interface_vnet 0, and the network is powered off. A tenant sandbox posts isolateon on the copy. true is the opt-in: a VM sandbox requires external_network_id and powers the network on, and a tenant sandbox is not isolated."
  default     = false
}

variable "vms" {
  type = list(object({
    key  = number
    name = string
    snapshots = list(object({
      id      = string
      name    = string
      created = number
    }))
  }))
  description = "VM inventory to clone. Omit it to read VMs assigned to tag and each VM's snapshots. Set it to skip those reads. tag must be null when this is set."
  default     = null

  validation {
    condition = var.vms == null || (
      length(var.vms) == length(distinct([for vm in var.vms : vm.key])) &&
      alltrue([for vm in var.vms : vm.key == floor(vm.key) && vm.key > 0 && length(vm.name) > 0])
    )
    error_message = "vms keys must be unique positive whole numbers, and each VM needs a name."
  }
}

variable "external_network_id" {
  type        = number
  description = "External network key written to vnet_default_gateway when allow_external is true and source_type is vm. Omit it when the sandbox stays isolated."
  default     = null

  validation {
    condition     = var.external_network_id == null || (var.external_network_id > 0 && var.external_network_id == floor(var.external_network_id))
    error_message = "external_network_id must be a positive whole number."
  }
}
