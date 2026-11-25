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

# One workspace, or one Stacks deployment, sets system_key and the credentials
# for that system. host comes from the inventory. Credentials come from the
# environment: VERGEOS_API_KEY, or VERGEOS_USERNAME and VERGEOS_PASSWORD.
provider "vergeio" {
  host = local.systems[var.system_key].host
}

variable "system_key" {
  type        = string
  description = "Inventory key of the system this workspace configures."
}

variable "waves" {
  type        = set(string)
  description = "Waves to apply on this system. Start with [\"canary\"]. Add \"rest\" after that plan matches."
  default     = ["canary"]
}

variable "login_lockout_attempts_key" {
  type        = string
  description = "VergeOS key for Failed login attempts before an account is locked. Read it on this system."
}

variable "login_lockout_minutes_key" {
  type        = string
  description = "VergeOS key for Minutes an account is locked for. Read it on this system."
}

variable "session_expiration_key" {
  type        = string
  description = "VergeOS key for Session expiration (seconds). Read it on this system."
}

variable "session_inactivity_key" {
  type        = string
  description = "VergeOS key for Session inactivity timeout (seconds). Read it on this system."
}

variable "logon_banner_header_key" {
  type        = string
  description = "VergeOS key for Logon banner header. Read it on this system."
}

variable "logon_banner_text_key" {
  type        = string
  description = "VergeOS key for Logon banner text. Read it on this system."
}

variable "password_complexity_key" {
  type        = string
  description = "VergeOS key for Password Complexity Requirement. Read it on this system."
}

locals {
  networks = {
    app = {
      description     = "Application segment"
      powerstate      = false
      dhcp_enabled    = true
      dynamic_dhcp    = true
      dhcp_sequential = true
      firewall_rules = [{
        name              = "allow-ssh"
        description       = "SSH from management"
        protocol          = "tcp"
        direction         = "incoming"
        action            = "accept"
        destination_ports = "22"
      }]
    }
  }

  systems = {
    lab = {
      host = "lab.example.com"
      tenants = {
        canary = {
          wave        = "canary"
          name        = "lab-canary"
          description = "Lab canary"
          powerstate  = false
          nodes = [{
            name      = "lab-canary-node1"
            cpu_cores = 2
            ram       = 2048
          }]
          storage = [{
            tier        = 1
            provisioned = 1073741824
          }]
          admin_user = {
            name        = "lab-canary-admin"
            displayname = "Lab canary admin"
          }
          networks = {
            app = {
              cidr       = "10.83.10.0/24"
              dhcp_start = "10.83.10.10"
              dhcp_stop  = "10.83.10.50"
              domain     = "lab-canary.example.com"
            }
          }
        }
        east = {
          wave        = "rest"
          name        = "lab-east"
          description = "Lab east"
          powerstate  = false
          nodes = [{
            name      = "lab-east-node1"
            cpu_cores = 2
            ram       = 2048
          }]
          storage = [{
            tier        = 1
            provisioned = 1073741824
          }]
          admin_user = {
            name        = "lab-east-admin"
            displayname = "Lab east admin"
          }
          networks = {
            app = {
              cidr       = "10.83.11.0/24"
              dhcp_start = "10.83.11.10"
              dhcp_stop  = "10.83.11.50"
              domain     = "lab-east.example.com"
            }
          }
        }
      }
    }
    edge = {
      host = "edge.example.com"
      tenants = {
        canary = {
          wave        = "canary"
          name        = "edge-canary"
          description = "Edge canary"
          powerstate  = false
          nodes = [{
            name      = "edge-canary-node1"
            cpu_cores = 2
            ram       = 2048
          }]
          storage = [{
            tier        = 1
            provisioned = 1073741824
          }]
          admin_user = {
            name        = "edge-canary-admin"
            displayname = "Edge canary admin"
          }
          networks = {
            app = {
              cidr       = "10.83.20.0/24"
              dhcp_start = "10.83.20.10"
              dhcp_stop  = "10.83.20.50"
              domain     = "edge-canary.example.com"
            }
          }
        }
        east = {
          wave        = "rest"
          name        = "edge-east"
          description = "Edge east"
          powerstate  = false
          nodes = [{
            name      = "edge-east-node1"
            cpu_cores = 2
            ram       = 2048
          }]
          storage = [{
            tier        = 1
            provisioned = 1073741824
          }]
          admin_user = {
            name        = "edge-east-admin"
            displayname = "Edge east admin"
          }
          networks = {
            app = {
              cidr       = "10.83.21.0/24"
              dhcp_start = "10.83.21.10"
              dhcp_stop  = "10.83.21.50"
              domain     = "edge-east.example.com"
            }
          }
        }
      }
    }
  }
}

resource "random_password" "tenant" {
  for_each = local.systems[var.system_key].tenants

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
  for_each = local.systems[var.system_key].tenants

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
  source = "../../../modules/fleet"

  system_key = var.system_key
  waves      = var.waves
  networks   = local.networks
  tenants    = local.systems[var.system_key].tenants

  tenant_passwords = { for key, password in random_password.tenant : key => password.result }
  admin_passwords  = { for key, password in random_password.admin : key => password.result }

  tenant_admin_group = {
    name        = "tenant-admins"
    description = "Tenant admins"
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
  baseline_apply_settings = true

  baseline_settings = {
    login_lockout_attempts = {
      key   = var.login_lockout_attempts_key
      value = "0"
    }
    login_lockout_minutes = {
      key   = var.login_lockout_minutes_key
      value = "15"
    }
    session_expiration = {
      key   = var.session_expiration_key
      value = "259200"
    }
    session_inactivity = {
      key   = var.session_inactivity_key
      value = "86400"
    }
    logon_banner_header = {
      key   = var.logon_banner_header_key
      value = "Authorized use"
    }
    logon_banner_text = {
      key   = var.logon_banner_text_key
      value = "Authorized use only."
    }
    password_complexity = {
      key   = var.password_complexity_key
      value = "length:8"
    }
  }

  baseline_certificates = [{
    name        = "ui"
    type        = "self_signed"
    domain_name = "ui.example.com"
    description = "UI certificate"
    key_type    = "ecdsa"
  }]

  baseline_groups = [{
    name        = "operators"
    description = "Operators"
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
    name            = "ops"
    displayname     = "Ops"
    email           = "ops@example.com"
    type            = "normal"
    change_password = true
    groups          = ["operators"]
    api_keys = [{
      name          = "ci"
      description   = "CI runner"
      expires       = 1893456000
      ip_allow_list = "192.0.2.10"
    }]
  }]

  baseline_passwords = {
    ops = random_password.operator.result
  }
}

output "system_key" {
  description = "Inventory key of the system this state configures."
  value       = module.fleet.system_key
}

output "pending_tenant_keys" {
  description = "Inventory keys left for a later wave."
  value       = module.fleet.pending_tenant_keys
}

output "tenant_ids" {
  description = "Tenant keys for the waves in this state."
  value       = module.fleet.tenant_ids
}

output "tenant_ui_addresses" {
  description = "Tenant UI addresses. A second configuration uses one as its provider host."
  value       = module.fleet.tenant_ui_addresses
}

output "network_ids" {
  description = "Internal network keys for the selected tenants."
  value       = module.fleet.network_ids
}

output "baseline_profile" {
  description = "Baseline profile."
  value       = module.fleet.baseline_profile
}
