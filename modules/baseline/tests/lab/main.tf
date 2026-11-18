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

module "baseline" {
  source = "../.."

  # System settings change the parent system, and destroy restores each key's
  # default_value. This lab leaves every system setting unchanged.
  apply_settings = false

  certificates = [{
    name        = "tf-acc-baseline"
    type        = "self_signed"
    domain_name = "tf-acc-baseline.local"
    description = "tf-acc-baseline"
    key_type    = "ecdsa"
  }]

  groups = [
    {
      name        = "tf-acc-baseline-admins"
      description = "tf-acc-baseline-admins"
      enabled     = true
      permissions = [{
        table  = "groups"
        list   = true
        read   = true
        create = false
        modify = false
        delete = false
      }]
    },
    {
      name        = "tf-acc-baseline-operators"
      description = "tf-acc-baseline-operators"
      enabled     = true
      permissions = [{
        table  = "vms"
        list   = true
        read   = true
        create = false
        modify = false
        delete = false
      }]
    },
  ]

  users = [
    {
      name            = "tf-acc-baseline-admin"
      displayname     = "tf-acc-baseline-admin"
      email           = "tf-acc-baseline-admin@example.com"
      type            = "normal"
      change_password = true
      groups          = ["tf-acc-baseline-admins"]
      api_keys = [{
        name          = "tf-acc-baseline-ci"
        description   = "tf-acc-baseline-ci"
        expires       = 1893456000
        ip_allow_list = "192.0.2.10"
      }]
    },
    {
      name            = "tf-acc-baseline-operator"
      displayname     = "tf-acc-baseline-operator"
      email           = "tf-acc-baseline-operator@example.com"
      type            = "normal"
      change_password = true
      groups          = ["tf-acc-baseline-operators"]
    },
  ]

  passwords = {
    tf-acc-baseline-admin    = random_password.admin.result
    tf-acc-baseline-operator = random_password.operator.result
  }
}

output "user_ids" {
  value = module.baseline.user_ids
}

output "group_ids" {
  value = module.baseline.group_ids
}

output "certificate_ids" {
  value = module.baseline.certificate_ids
}

output "api_key_ids" {
  value = module.baseline.api_key_ids
}

output "setting_ids" {
  value = module.baseline.setting_ids
}
