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

variable "external_network_name" {
  type        = string
  description = "Name of the parent external network that hands the tenant its UI address and routed block."
  default     = "External"
}

variable "tenant_password" {
  type        = string
  description = "Password of the tenant's built-in admin user."
  sensitive   = true
}

variable "admin_password" {
  type        = string
  description = "Password of the parent-system admin user."
  sensitive   = true
}

data "vergeio_groups" "admins" {
  filter_name = "Administrators (default)"
}

data "vergeio_networks" "external" {
  filter_name = var.external_network_name
  filter_type = "external"
}

locals {
  external_network_id = tostring(data.vergeio_networks.external.networks[0].id)
}

module "tenant" {
  source = "../../../modules/tenant"

  name                       = "customer-a"
  description                = "Customer A virtual data center"
  tenant_password_wo         = var.tenant_password
  tenant_password_wo_version = 1
  powerstate                 = true
  admin_group_id             = data.vergeio_groups.admins.groups[0].id
  admin_password_wo          = var.admin_password
  admin_password_wo_version  = 1

  admin_user = {
    name        = "customer-a-admin"
    displayname = "Customer A admin"
    email       = "customer-a-admin@example.com"
  }

  nodes = [{
    name      = "node1"
    cpu_cores = 4
    ram       = 8192
    enabled   = true
  }]

  storage = [{
    tier        = 1
    provisioned = 107374182400
  }]

  network_blocks = [{
    network_id  = local.external_network_id
    cidr        = "198.51.100.0/28"
    description = "Customer A routed addresses"
  }]

  external_ips = [{
    network_id  = local.external_network_id
    ip          = "203.0.113.50"
    hostname    = "customer-a"
    description = "Tenant UI address"
  }]
}

output "tenant_id" {
  description = "Tenant key."
  value       = module.tenant.id
}

output "tenant_ui_address" {
  description = "Tenant UI address for a stack that manages the inside of the tenant."
  value       = module.tenant.ui_address
}

output "admin_user_id" {
  description = "Parent-system admin user key."
  value       = module.tenant.admin_user_id
}
