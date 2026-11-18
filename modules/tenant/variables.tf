variable "name" {
  type        = string
  description = "Tenant name. Must be unique on the parent system."

  validation {
    condition     = length(var.name) > 0
    error_message = "name must not be empty."
  }
}

variable "description" {
  type        = string
  description = "Tenant description. Omit to leave an existing description unchanged."
  default     = null
}

variable "tenant_password_wo" {
  type        = string
  description = "Password of the tenant's built-in admin user. Sent as vergeio_tenant.password_wo and not stored. VergeOS requires an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true

  validation {
    condition = (
      length(var.tenant_password_wo) > 0 &&
      can(regex("[A-Z]", var.tenant_password_wo)) &&
      can(regex("[a-z]", var.tenant_password_wo)) &&
      can(regex("[0-9]", var.tenant_password_wo))
    )
    error_message = "tenant_password_wo must include an uppercase letter, a lowercase letter, and a digit."
  }
}

variable "tenant_password_wo_version" {
  type        = number
  description = "Increment to change tenant_password_wo. Terraform stores this number, not the password."
  default     = 1

  validation {
    condition     = var.tenant_password_wo_version >= 1 && var.tenant_password_wo_version == floor(var.tenant_password_wo_version)
    error_message = "tenant_password_wo_version must be a whole number of at least 1."
  }
}

variable "change_password" {
  type        = bool
  description = "Require a password change on first login. Sent only when the tenant is created."
  default     = null
}

variable "powerstate" {
  type        = bool
  description = "Whether the tenant is powered on. True defers power on until the tenant has a node. Omit to leave the current power unchanged."
  default     = null
}

variable "isolate" {
  type        = bool
  description = "Whether tenant network isolation is enabled. Omit to leave the current isolation unchanged."
  default     = null
}

variable "nodes" {
  type = list(object({
    name           = string
    cpu_cores      = number
    ram            = number
    description    = optional(string)
    enabled        = optional(bool, true)
    on_power_loss  = optional(string)
    cluster        = optional(number)
    preferred_node = optional(number)
  }))
  description = "Nodes handed to the tenant. ram is megabytes. At least one node is required."

  validation {
    condition     = length(var.nodes) > 0
    error_message = "nodes must contain at least one node."
  }

  validation {
    condition     = length(var.nodes) == length(distinct([for node in var.nodes : node.name]))
    error_message = "node names must be unique."
  }

  validation {
    condition = alltrue([
      for node in var.nodes :
      length(node.name) > 0 && node.cpu_cores >= 1 && node.cpu_cores == floor(node.cpu_cores) && node.ram >= 2048 && node.ram == floor(node.ram)
    ])
    error_message = "Each node needs a name, a whole cpu_cores of at least 1, and a whole ram of at least 2048 megabytes."
  }

  validation {
    condition = alltrue([
      for node in var.nodes :
      node.on_power_loss == null || contains(["power_on", "last_state", "leave_off"], node.on_power_loss)
    ])
    error_message = "node on_power_loss must be power_on, last_state, or leave_off."
  }
}

variable "storage" {
  type = list(object({
    tier        = number
    provisioned = number
  }))
  description = "Storage allocations. provisioned is bytes and must be a positive multiple of 1073741824 (1 GiB). tier is the storage tier key."

  validation {
    condition     = length(var.storage) > 0
    error_message = "storage must contain at least one allocation."
  }

  validation {
    condition     = length(var.storage) == length(distinct([for item in var.storage : item.tier]))
    error_message = "storage tiers must be unique. Changing tier replaces the allocation."
  }

  validation {
    condition = alltrue([
      for item in var.storage :
      item.tier == floor(item.tier) && item.provisioned > 0 && item.provisioned == floor(item.provisioned) && item.provisioned % 1073741824 == 0
    ])
    error_message = "Each storage tier must be a whole number, and provisioned must be a positive multiple of 1073741824 (1 GiB)."
  }
}

variable "network_blocks" {
  type = list(object({
    network_id            = string
    cidr                  = string
    description           = optional(string)
    apply_parent_firewall = optional(bool, true)
  }))
  description = "Routed CIDRs from a parent network. cidr must be the network address. network_id is vergeio_network.id. Omit to assign none."
  default     = []

  validation {
    condition     = length(var.network_blocks) == length(distinct([for block in var.network_blocks : block.cidr]))
    error_message = "network block CIDRs must be unique."
  }

  validation {
    condition = alltrue([
      for block in var.network_blocks :
      length(block.network_id) > 0 && can(cidrhost(block.cidr, 0)) && try(cidrhost(block.cidr, 0) == split("/", block.cidr)[0], false)
    ])
    error_message = "Each network block needs a network_id and a network address in CIDR notation, such as 198.51.100.0/28."
  }
}

variable "external_ips" {
  type = list(object({
    network_id            = string
    ip                    = string
    hostname              = optional(string)
    description           = optional(string)
    apply_parent_firewall = optional(bool, true)
  }))
  description = "Virtual IPs on a parent network. The first assigned IP becomes the tenant UI address. Omit to assign none."
  default     = []

  validation {
    condition     = length(var.external_ips) == length(distinct([for addr in var.external_ips : addr.ip]))
    error_message = "external IP addresses must be unique."
  }

  validation {
    condition = alltrue([
      for addr in var.external_ips :
      length(addr.network_id) > 0 && can(cidrhost("${addr.ip}/32", 0))
    ])
    error_message = "Each external IP needs a network_id and an IP address."
  }
}

variable "admin_group_id" {
  type        = number
  description = "Group key the admin user joins. vergeio_groups id is this number. vergeio_member.group uses it."

  validation {
    condition     = var.admin_group_id == floor(var.admin_group_id)
    error_message = "admin_group_id must be a whole number."
  }
}

variable "admin_password_wo" {
  type        = string
  description = "Password of the parent-system admin user. Sent as vergeio_user.password_wo and not stored. VergeOS requires an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true

  validation {
    condition = (
      length(var.admin_password_wo) > 0 &&
      can(regex("[A-Z]", var.admin_password_wo)) &&
      can(regex("[a-z]", var.admin_password_wo)) &&
      can(regex("[0-9]", var.admin_password_wo))
    )
    error_message = "admin_password_wo must include an uppercase letter, a lowercase letter, and a digit."
  }
}

variable "admin_password_wo_version" {
  type        = number
  description = "Increment to change admin_password_wo. Terraform stores this number, not the password."
  default     = 1

  validation {
    condition     = var.admin_password_wo_version >= 1 && var.admin_password_wo_version == floor(var.admin_password_wo_version)
    error_message = "admin_password_wo_version must be a whole number of at least 1."
  }
}

variable "admin_user" {
  type = object({
    name            = string
    displayname     = optional(string)
    email           = optional(string)
    enabled         = optional(bool, true)
    type            = optional(string, "normal")
    change_password = optional(bool, true)
  })
  description = "Parent-system user created with this tenant and added to admin_group_id."

  validation {
    condition     = length(var.admin_user.name) > 0
    error_message = "admin_user.name must not be empty."
  }

  validation {
    condition     = contains(["normal", "api", "vdi"], var.admin_user.type)
    error_message = "admin_user.type must be normal, api, or vdi."
  }
}
