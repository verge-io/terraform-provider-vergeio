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

variable "tenant_password" {
  type      = string
  sensitive = true
}

variable "admin_password" {
  type      = string
  sensitive = true
}

resource "vergeio_group" "admins" {
  name        = "tf-acc-tenant-admins"
  description = "tf-acc-tenant-admins"
  enabled     = true
}

module "tenant" {
  source = "../.."

  name                       = "tf-acc-tenant"
  description                = "tf-acc-tenant"
  tenant_password_wo         = var.tenant_password
  tenant_password_wo_version = 1
  powerstate                 = false
  admin_group_id             = tonumber(vergeio_group.admins.id)
  admin_password_wo          = var.admin_password
  admin_password_wo_version  = 1

  admin_user = {
    name        = "tf-acc-tenant-admin"
    displayname = "tf-acc-tenant-admin"
  }

  nodes = [{
    name      = "tf-acc-node1"
    cpu_cores = 2
    ram       = 2048
  }]

  storage = [{
    tier        = 1
    provisioned = 1073741824
  }]
}

output "tenant_id" {
  value = module.tenant.id
}

output "admin_user_id" {
  value = module.tenant.admin_user_id
}
