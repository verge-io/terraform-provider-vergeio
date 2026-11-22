terraform {
  required_version = ">= 1.11.0"

  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "vergeio" {}

variable "name" {
  type        = string
  description = "Sandbox name. A pull request workflow can pass pr-<number>."
}

variable "source_type" {
  type        = string
  description = "tenant or vm."
}

variable "tenant_id" {
  type        = string
  description = "Source tenant key when source_type is tenant."
  default     = null
}

variable "tag" {
  type        = string
  description = "Tag name or category/name when source_type is vm."
  default     = null
}

variable "cidr" {
  type        = string
  description = "Sandbox network CIDR when source_type is vm."
  default     = null
}

variable "snapshot_name" {
  type        = string
  description = "Snapshot name. Omit to use the newest snapshot."
  default     = null
}

variable "allow_external" {
  type        = bool
  description = "Opt in to a route outside the sandbox."
  default     = false
}

variable "external_network_id" {
  type        = number
  description = "External network key. Used only when allow_external is true and source_type is vm."
  default     = null
}

module "sandbox" {
  source = "../../../modules/sandbox"

  name                = var.name
  source_type         = var.source_type
  tenant_id           = var.tenant_id
  tag                 = var.tag
  cidr                = var.cidr
  snapshot_name       = var.snapshot_name
  allow_external      = var.allow_external
  external_network_id = var.external_network_id
}

output "network_id" {
  description = "Sandbox network key. Null for a tenant sandbox."
  value       = module.sandbox.network_id
}

output "tenant_id" {
  description = "Copied tenant key. Null for a VM sandbox."
  value       = module.sandbox.tenant_id
}

output "vm_ids" {
  description = "Copied VM keys, keyed by the source VM key."
  value       = module.sandbox.vm_ids
}

output "isolated" {
  description = "True when the sandbox was not given an external route."
  value       = module.sandbox.isolated
}
