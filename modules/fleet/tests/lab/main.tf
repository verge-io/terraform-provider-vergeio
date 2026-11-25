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
  for_each = toset(["canary", "later"])

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
  for_each = toset(["canary", "later"])

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

resource "random_password" "operator" {
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

module "fleet" {
  source = "../.."

  # The later tenant stays in the inventory and is not created. waves selects
  # the canary. baseline_apply_settings false leaves every system setting
  # unchanged. Nothing here reads an existing vnet, tenant, user, or group.
  system_key = "lab"
  waves      = ["canary"]

  networks = {
    app = {
      description     = "tf-acc-fleet-app"
      enabled         = true
      powerstate      = false
      dhcp_enabled    = true
      dynamic_dhcp    = true
      dhcp_sequential = true
      firewall_rules = [{
        name              = "tf-acc-allow-ssh"
        description       = "tf-acc-allow-ssh"
        protocol          = "tcp"
        direction         = "incoming"
        action            = "accept"
        destination_ports = "22"
      }]
    }
  }

  tenants = {
    canary = {
      wave        = "canary"
      name        = "tf-acc-fleet-canary"
      description = "tf-acc-fleet-canary"
      powerstate  = false
      nodes = [{
        name        = "tf-acc-fleet-canary-node"
        description = "tf-acc-fleet-canary-node"
        cpu_cores   = 2
        ram         = 2048
      }]
      storage = [{
        tier        = 1
        provisioned = 1073741824
      }]
      admin_user = {
        name        = "tf-acc-fleet-canary-admin"
        displayname = "tf-acc-fleet-canary-admin"
        email       = "tf-acc-fleet-canary-admin@example.com"
      }
      networks = {
        app = {
          cidr       = "10.83.10.0/24"
          dhcp_start = "10.83.10.10"
          dhcp_stop  = "10.83.10.50"
          domain     = "tf-acc-fleet-canary.example.com"
        }
      }
    }
    later = {
      wave        = "rest"
      name        = "tf-acc-fleet-later"
      description = "tf-acc-fleet-later"
      powerstate  = false
      nodes = [{
        name        = "tf-acc-fleet-later-node"
        description = "tf-acc-fleet-later-node"
        cpu_cores   = 2
        ram         = 2048
      }]
      storage = [{
        tier        = 1
        provisioned = 1073741824
      }]
      admin_user = {
        name        = "tf-acc-fleet-later-admin"
        displayname = "tf-acc-fleet-later-admin"
        email       = "tf-acc-fleet-later-admin@example.com"
      }
      networks = {
        app = {
          cidr       = "10.83.11.0/24"
          dhcp_start = "10.83.11.10"
          dhcp_stop  = "10.83.11.50"
          domain     = "tf-acc-fleet-later.example.com"
        }
      }
    }
  }

  tenant_passwords = {
    canary = random_password.tenant["canary"].result
    later  = random_password.tenant["later"].result
  }

  admin_passwords = {
    canary = random_password.admin["canary"].result
    later  = random_password.admin["later"].result
  }

  tenant_admin_group = {
    name        = "tf-acc-fleet-admins"
    description = "tf-acc-fleet-admins"
    permissions = [{
      table  = "tenants"
      list   = true
      read   = true
      create = false
      modify = false
      delete = false
    }]
  }

  baseline_profile        = "strict"
  baseline_apply_settings = false

  baseline_certificates = [{
    name        = "tf-acc-fleet"
    type        = "self_signed"
    domain_name = "tf-acc-fleet.local"
    description = "tf-acc-fleet"
    key_type    = "ecdsa"
  }]

  baseline_groups = [{
    name        = "tf-acc-fleet-operators"
    description = "tf-acc-fleet-operators"
    permissions = [{
      table  = "vms"
      list   = true
      read   = true
      create = false
      modify = false
      delete = false
    }]
  }]

  baseline_users = [{
    name            = "tf-acc-fleet-operator"
    displayname     = "tf-acc-fleet-operator"
    email           = "tf-acc-fleet-operator@example.com"
    type            = "normal"
    change_password = true
    groups          = ["tf-acc-fleet-operators"]
    api_keys = [{
      name          = "tf-acc-fleet-ci"
      description   = "tf-acc-fleet-ci"
      expires       = 1893456000
      ip_allow_list = "192.0.2.10"
    }]
  }]

  baseline_passwords = {
    tf-acc-fleet-operator = random_password.operator.result
  }
}

output "tenant_ids" {
  value = module.fleet.tenant_ids
}

output "pending_tenant_keys" {
  value = module.fleet.pending_tenant_keys
}

output "network_ids" {
  value = module.fleet.network_ids
}

output "network_names" {
  value = module.fleet.network_names
}

output "network_addresses" {
  value = module.fleet.network_addresses
}

output "tenant_admin_group_id" {
  value = module.fleet.tenant_admin_group_id
}

output "baseline_user_ids" {
  value = module.fleet.baseline_user_ids
}

output "baseline_certificate_ids" {
  value = module.fleet.baseline_certificate_ids
}

output "baseline_setting_ids" {
  value = module.fleet.baseline_setting_ids
}

output "baseline_api_key_ids" {
  value = module.fleet.baseline_api_key_ids
}
