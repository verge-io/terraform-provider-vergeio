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

variable "login_lockout_attempts_key" {
  type        = string
  description = "VergeOS key for Failed login attempts before an account is locked. Read it on the system."
}

variable "login_lockout_minutes_key" {
  type        = string
  description = "VergeOS key for Minutes an account is locked for. Read it on the system."
}

variable "session_expiration_key" {
  type        = string
  description = "VergeOS key for Session expiration (seconds). Read it on the system."
}

variable "session_inactivity_key" {
  type        = string
  description = "VergeOS key for Session inactivity timeout (seconds). Read it on the system."
}

variable "logon_banner_header_key" {
  type        = string
  description = "VergeOS key for Logon banner header. Read it on the system."
}

variable "logon_banner_text_key" {
  type        = string
  description = "VergeOS key for Logon banner text. Read it on the system."
}

variable "password_complexity_key" {
  type        = string
  description = "VergeOS key for Password Complexity Requirement. Read it on the system."
}

variable "login_lockout_attempts" {
  type        = string
  description = "Failed login attempts before an account is locked. The product-guide default is 0, which disables lockout."
  default     = "0"
}

variable "login_lockout_minutes" {
  type        = string
  description = "Minutes an account is locked for. The product-guide default is 15."
  default     = "15"
}

variable "session_expiration" {
  type        = string
  description = "Session expiration in seconds. The product-guide default is 259200."
  default     = "259200"
}

variable "session_inactivity" {
  type        = string
  description = "Session inactivity timeout in seconds. The product-guide default is 86400."
  default     = "86400"
}

variable "logon_banner_header" {
  type        = string
  description = "Logon banner header. The product-guide default is empty."
  default     = ""
}

variable "logon_banner_text" {
  type        = string
  description = "Logon banner text. The product-guide default is empty."
  default     = ""
}

variable "password_complexity" {
  type        = string
  description = "Password Complexity Requirement. The product-guide default is length:8."
  default     = "length:8"
}

resource "random_password" "ops" {
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
  source = "../../../modules/baseline"

  profile = "strict"

  settings = {
    login_lockout_attempts = {
      key   = var.login_lockout_attempts_key
      value = var.login_lockout_attempts
    }
    login_lockout_minutes = {
      key   = var.login_lockout_minutes_key
      value = var.login_lockout_minutes
    }
    session_expiration = {
      key   = var.session_expiration_key
      value = var.session_expiration
    }
    session_inactivity = {
      key   = var.session_inactivity_key
      value = var.session_inactivity
    }
    logon_banner_header = {
      key   = var.logon_banner_header_key
      value = var.logon_banner_header
    }
    logon_banner_text = {
      key   = var.logon_banner_text_key
      value = var.logon_banner_text
    }
    password_complexity = {
      key   = var.password_complexity_key
      value = var.password_complexity
    }
  }

  certificates = [{
    name        = "ui"
    type        = "self_signed"
    domain_name = "ui.example.com"
    description = "UI certificate"
    key_type    = "ecdsa"
  }]

  groups = [{
    name        = "operators"
    description = "Operators"
    enabled     = true
    permissions = [{
      table  = "vms"
      list   = true
      read   = true
      create = false
      modify = false
      delete = false
    }]
  }]

  users = [{
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

  passwords = {
    ops = random_password.ops.result
  }
}

output "profile" {
  description = "Selected profile."
  value       = module.baseline.profile
}

output "controls" {
  description = "Control mapping."
  value       = module.baseline.controls
}

output "user_id" {
  description = "Ops user key."
  value       = module.baseline.user_ids["ops"]
}

output "certificate_id" {
  description = "UI certificate key."
  value       = module.baseline.certificate_ids["ui"]
}
