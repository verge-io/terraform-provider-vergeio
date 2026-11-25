terraform {
  required_version = ">= 1.11.0"

  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "random" {}

provider "vergeio" {}

resource "random_password" "tenant" {
  length           = 24
  upper            = true
  lower            = true
  numeric          = true
  special          = true
  override_special = "!@#%^*-_"
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
}

resource "random_password" "admin" {
  length           = 24
  upper            = true
  lower            = true
  numeric          = true
  special          = true
  override_special = "!@#%^*-_"
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
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
  tenant_password_wo         = random_password.tenant.result
  tenant_password_wo_version = 1
  powerstate                 = false
  admin_group_id             = tonumber(vergeio_group.admins.id)
  admin_password_wo          = random_password.admin.result
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
