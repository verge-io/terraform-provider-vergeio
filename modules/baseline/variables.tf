variable "profile" {
  type        = string
  description = "Named profile. baseline writes the controls you pass. strict also requires every setting control, a certificate, a group, and a user in a group."
  default     = "baseline"

  validation {
    condition     = contains(["baseline", "strict"], var.profile)
    error_message = "profile must be baseline or strict."
  }
}

variable "apply_settings" {
  type        = bool
  description = "Write vergeio_setting for each control in settings. Set false to leave every system setting unchanged. Destroy of a setting this module writes restores that key's default_value."
  default     = true
}

variable "settings" {
  type = map(object({
    key   = string
    value = string
  }))
  description = "System settings to write, keyed by control id. key is the string VergeOS stores. value is stored in state. for_each uses the control id, which must be known at plan time. An empty string is a real value."
  default     = {}

  validation {
    condition = alltrue([
      for id, spec in var.settings :
      length(spec.key) > 0 && length(regexall("[/\\\\?#\\s]", spec.key)) == 0
    ])
    error_message = "Each setting key must be a non-empty VergeOS key without spaces or / \\ ? #."
  }

  validation {
    condition     = length(var.settings) == length(distinct([for id, spec in var.settings : spec.key]))
    error_message = "Setting keys must be unique. Two controls cannot write the same VergeOS key."
  }
}

variable "api_key_max_lifetime_seconds" {
  type        = number
  description = "When set, each API key expires must fall within this many seconds of the plan time. VergeOS stores expiry on the key. There is no system setting for a default lifetime."
  default     = null

  validation {
    condition     = var.api_key_max_lifetime_seconds == null || (var.api_key_max_lifetime_seconds >= 1 && var.api_key_max_lifetime_seconds == floor(var.api_key_max_lifetime_seconds))
    error_message = "api_key_max_lifetime_seconds must be a whole number of at least 1."
  }
}

variable "certificates" {
  type = list(object({
    name                    = string
    type                    = string
    domain_name             = optional(string)
    domain_list             = optional(string)
    description             = optional(string)
    public_certificate      = optional(string)
    private_key_wo_version  = optional(number)
    chain                   = optional(string)
    acme_server             = optional(string)
    eab_kid                 = optional(string)
    eab_hmac_key_wo_version = optional(number)
    key_type                = optional(string)
    rsa_key_size            = optional(string)
    contact                 = optional(number)
    agree_tos               = optional(bool)
  }))
  description = "TLS certificates to create. name is the for_each key and must be known at plan time. type is manual, letsencrypt, or self_signed. A manual private key goes in certificate_private_keys. Creating a certificate stores the row. It does not select the certificate the UI serves."
  default     = []

  validation {
    condition     = length(var.certificates) == length(distinct([for cert in var.certificates : cert.name]))
    error_message = "Certificate names must be unique."
  }

  validation {
    condition = alltrue([
      for cert in var.certificates :
      length(cert.name) > 0 && contains(["manual", "letsencrypt", "self_signed"], cert.type)
    ])
    error_message = "Each certificate needs a name and a type of manual, letsencrypt, or self_signed."
  }

  validation {
    condition = alltrue([
      for cert in var.certificates :
      cert.key_type == null || contains(["ecdsa", "rsa"], cert.key_type)
    ])
    error_message = "Certificate key_type must be ecdsa or rsa."
  }

  validation {
    condition = alltrue([
      for cert in var.certificates :
      cert.type != "self_signed" || (cert.domain_name != null && length(cert.domain_name) > 0)
    ])
    error_message = "A self_signed certificate needs domain_name."
  }

  validation {
    condition = alltrue([
      for cert in var.certificates :
      cert.type != "manual" || (
        cert.public_certificate != null && length(cert.public_certificate) > 0 &&
        cert.private_key_wo_version != null && cert.private_key_wo_version >= 1 && cert.private_key_wo_version == floor(cert.private_key_wo_version)
      )
    ])
    error_message = "A manual certificate needs public_certificate and a whole private_key_wo_version of at least 1. Put the private key in certificate_private_keys."
  }

  validation {
    condition = alltrue([
      for cert in var.certificates :
      cert.type != "letsencrypt" || (
        cert.agree_tos == true && cert.contact != null && cert.contact >= 1 && cert.contact == floor(cert.contact) &&
        ((cert.domain_name != null && length(cert.domain_name) > 0) || (cert.domain_list != null && length(cert.domain_list) > 0))
      )
    ])
    error_message = "A letsencrypt certificate needs agree_tos true, a contact user id, and domain_name or domain_list."
  }

  validation {
    condition = alltrue([
      for cert in var.certificates :
      cert.eab_hmac_key_wo_version == null || (cert.eab_hmac_key_wo_version >= 1 && cert.eab_hmac_key_wo_version == floor(cert.eab_hmac_key_wo_version))
    ])
    error_message = "eab_hmac_key_wo_version must be a whole number of at least 1."
  }
}

variable "certificate_private_keys" {
  type        = map(string)
  description = "PEM private keys for manual certificates, keyed by certificate name. Sent as private_key_wo and not stored. VergeOS stores the version on the certificate."
  sensitive   = true
  default     = {}
}

variable "certificate_eab_hmac_keys" {
  type        = map(string)
  description = "External account binding HMAC keys for ACME certificates, keyed by certificate name. Sent as eab_hmac_key_wo and not stored."
  sensitive   = true
  default     = {}
}

variable "groups" {
  type = list(object({
    name        = string
    description = optional(string)
    enabled     = optional(bool, true)
    permissions = optional(list(object({
      table     = string
      object_id = optional(number)
      list      = optional(bool)
      read      = optional(bool)
      create    = optional(bool)
      modify    = optional(bool)
      delete    = optional(bool)
    })), [])
  }))
  description = "Groups this module creates, and the vergeio_permission grants on each group. name is the for_each key. This module creates these groups. A membership in a group that already exists is a vergeio_member next to the module."
  default     = []

  validation {
    condition     = length(var.groups) == length(distinct([for group in var.groups : group.name]))
    error_message = "Group names must be unique."
  }

  validation {
    condition = alltrue([
      for group in var.groups :
      length(group.name) > 0 && length(regexall("[/\\s]", group.name)) == 0
    ])
    error_message = "Each group needs a name without spaces or slashes."
  }

  validation {
    condition = length(flatten([
      for group in var.groups : [
        for perm in group.permissions : "${group.name}/${perm.table}/${perm.object_id == null ? "table" : tostring(perm.object_id)}"
      ]
      ])) == length(distinct(flatten([
        for group in var.groups : [
          for perm in group.permissions : "${group.name}/${perm.table}/${perm.object_id == null ? "table" : tostring(perm.object_id)}"
        ]
    ])))
    error_message = "Each group can grant a table, or one object in that table, once."
  }

  validation {
    condition = alltrue([
      for group in var.groups : alltrue([
        for perm in group.permissions :
        length(perm.table) > 0 && (perm.read != true || perm.list == null || perm.list == true)
      ])
    ])
    error_message = "Each permission needs a table. When read is true, list must be true or omitted."
  }
}

variable "users" {
  type = list(object({
    name            = string
    displayname     = optional(string)
    email           = optional(string)
    enabled         = optional(bool, true)
    type            = optional(string, "normal")
    change_password = optional(bool, true)
    groups          = optional(list(string), [])
    api_keys = optional(list(object({
      name          = string
      description   = optional(string)
      expires       = number
      ip_allow_list = optional(string)
      ip_deny_list  = optional(string)
    })), [])
  }))
  description = "Users this module creates. name is the for_each key and must be known at plan time. groups names entries in groups. Each api_keys entry sets expires. A key with no expiry is a vergeio_api_key next to the module."
  default     = []

  validation {
    condition     = length(var.users) == length(distinct([for user in var.users : user.name]))
    error_message = "User names must be unique."
  }

  validation {
    condition = alltrue([
      for user in var.users :
      length(user.name) > 0 && length(regexall("[/\\s]", user.name)) == 0 && contains(["normal", "api", "vdi"], user.type)
    ])
    error_message = "Each user needs a name without spaces or slashes, and a type of normal, api, or vdi."
  }

  validation {
    condition = alltrue([
      for user in var.users :
      length(user.groups) == length(distinct(user.groups))
    ])
    error_message = "A user can be a member of a group once."
  }

  validation {
    condition = alltrue([
      for user in var.users :
      length(user.api_keys) == length(distinct([for key in user.api_keys : key.name]))
    ])
    error_message = "API key names must be unique for a user."
  }

  validation {
    condition = alltrue([
      for user in var.users : alltrue([
        for key in user.api_keys :
        length(key.name) > 0 && key.expires >= 1 && key.expires == floor(key.expires)
      ])
    ])
    error_message = "Each API key needs a name and a whole expires of at least 1. expires is the Unix time the key expires."
  }
}

variable "passwords" {
  type        = map(string)
  description = "Passwords keyed by user name. Sent as vergeio_user.password_wo and not stored. VergeOS requires an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true
  default     = {}

  validation {
    condition = alltrue([
      for name, password in var.passwords :
      length(password) >= 8 && can(regex("[A-Z]", password)) && can(regex("[a-z]", password)) && can(regex("[0-9]", password))
    ])
    error_message = "Each password must be at least 8 characters and include an uppercase letter, a lowercase letter, and a digit."
  }
}

variable "password_versions" {
  type        = map(number)
  description = "password_wo_version keyed by user name. Omit a name to use 1. Increment the number to change that user's password."
  default     = {}

  validation {
    condition = alltrue([
      for name, version in var.password_versions :
      version >= 1 && version == floor(version)
    ])
    error_message = "Each password version must be a whole number of at least 1."
  }
}
