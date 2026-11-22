terraform {
  required_version = ">= 1.14.0"

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

variable "enable_sandbox" {
  type    = bool
  default = false
}

resource "random_password" "tenant" {
  length      = 24
  upper       = true
  lower       = true
  numeric     = true
  min_upper   = 1
  min_lower   = 1
  min_numeric = 1
}

resource "vergeio_vm" "source" {
  name        = "tf-acc-sandbox-src"
  description = "tf-acc-sandbox-src"
  powerstate  = false
  cpu_cores   = 1
  ram         = 1024

  boot_disk {
    name = "tf-acc-sandbox-disk"
    size = 8
  }

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.vergeio_vm_snapshot.source]
    }
  }
}

action "vergeio_vm_snapshot" "source" {
  config {
    vm_id             = vergeio_vm.source.id
    name              = "tf-acc-sandbox-src"
    retention_seconds = 86400
  }
}

resource "vergeio_tag_category" "role" {
  name         = "tf-acc-sandbox-role"
  description  = "tf-acc-sandbox-role"
  taggable_vms = true
}

resource "vergeio_tag" "app" {
  category    = tonumber(vergeio_tag_category.role.id)
  name        = "tf-acc-sandbox"
  description = "tf-acc-sandbox"
}

resource "vergeio_tag_member" "source" {
  tag_id = tonumber(vergeio_tag.app.id)
  member = "vms/${vergeio_vm.source.id}"
}

resource "vergeio_tenant" "source" {
  name                = "tf-acc-sandbox-tenant"
  description         = "tf-acc-sandbox-tenant"
  password_wo         = random_password.tenant.result
  password_wo_version = 1
  powerstate          = false

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.vergeio_tenant_snapshot.source]
    }
  }
}

action "vergeio_tenant_snapshot" "source" {
  config {
    tenant_id         = vergeio_tenant.source.id
    name              = "tf-acc-sandbox-tenant"
    retention_seconds = 86400
  }
}

module "vms" {
  count  = var.enable_sandbox ? 1 : 0
  source = "../.."

  name          = "tf-acc-sandbox"
  source_type   = "vm"
  tag           = "tf-acc-sandbox-role/tf-acc-sandbox"
  snapshot_name = "tf-acc-sandbox-src"
  cidr          = "192.168.91.0/24"

  depends_on = [vergeio_tag_member.source]
}

module "tenant" {
  count  = var.enable_sandbox ? 1 : 0
  source = "../.."

  name          = "tf-acc-sandbox-copy"
  source_type   = "tenant"
  tenant_id     = vergeio_tenant.source.id
  snapshot_name = "tf-acc-sandbox-tenant"
}

output "vm_ids" {
  value = try(module.vms[0].vm_ids, {})
}

output "network_id" {
  value = try(module.vms[0].network_id, null)
}

output "tenant_id" {
  value = try(module.tenant[0].tenant_id, null)
}

output "isolated" {
  value = var.enable_sandbox ? module.vms[0].isolated && module.tenant[0].isolated : true
}
