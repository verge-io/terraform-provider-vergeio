locals {
  # Control ids are the for_each keys. The caller supplies the VergeOS key.
  # apply_settings false leaves every system setting unchanged.
  setting_catalog = {
    login_lockout_attempts = {
      title            = "Login lockout threshold"
      guide            = "Failed login attempts before an account is locked (0 disables lockout)"
      documented_value = "0"
      strict           = true
    }
    login_lockout_minutes = {
      title            = "Login lockout duration"
      guide            = "Minutes an account is locked for"
      documented_value = "15"
      strict           = true
    }
    session_expiration = {
      title            = "Session expiration"
      guide            = "Session expiration (seconds)"
      documented_value = "259200"
      strict           = true
    }
    session_inactivity = {
      title            = "Session inactivity timeout"
      guide            = "Session inactivity timeout (seconds)"
      documented_value = "86400"
      strict           = true
    }
    logon_banner_header = {
      title            = "Logon banner header"
      guide            = "Logon banner header"
      documented_value = null
      strict           = true
    }
    logon_banner_text = {
      title            = "Logon banner text"
      guide            = "Logon banner text"
      documented_value = null
      strict           = true
    }
    password_complexity = {
      title            = "Password complexity"
      guide            = "Password Complexity Requirement"
      documented_value = "length:8"
      strict           = true
    }
  }

  other_controls = {
    tls_certificate = {
      title            = "TLS certificate"
      guide            = "TLS certificate for the UI and API"
      resource         = "vergeio_certificate"
      documented_value = null
      strict           = true
    }
    api_key_expiry = {
      title            = "API key expiry"
      guide            = "expires on each vergeio_api_key this module creates"
      resource         = "vergeio_api_key"
      documented_value = null
      strict           = false
    }
    admin_accounts = {
      title            = "Admin account layout"
      guide            = "Users, groups, memberships, and permissions this module creates"
      resource         = "vergeio_user"
      documented_value = null
      strict           = true
    }
  }

  applied_settings = var.apply_settings ? {
    for id, spec in var.settings : id => spec
    if contains(keys(local.setting_catalog), id)
  } : {}

  required_setting_ids = var.profile == "strict" && var.apply_settings ? toset([
    for id, control in local.setting_catalog : id if control.strict
  ]) : toset([])

  groups       = { for group in var.groups : group.name => group }
  users        = { for user in var.users : user.name => user }
  certificates = { for cert in var.certificates : cert.name => cert }

  permission_list = flatten([
    for group in var.groups : [
      for perm in group.permissions : {
        key        = "${group.name}/${perm.table}/${perm.object_id == null ? "table" : tostring(perm.object_id)}"
        group_name = group.name
        table      = perm.table
        object_id  = perm.object_id
        list       = perm.list
        read       = perm.read
        create     = perm.create
        modify     = perm.modify
        delete     = perm.delete
      }
    ]
  ])
  permissions = { for perm in local.permission_list : perm.key => perm }

  membership_list = flatten([
    for user in var.users : [
      for group_name in user.groups : {
        key        = "${user.name}/${group_name}"
        user_name  = user.name
        group_name = group_name
      }
      if contains(keys(local.groups), group_name)
    ]
  ])
  memberships = { for item in local.membership_list : item.key => item }

  api_key_list = flatten([
    for user in var.users : [
      for key in user.api_keys : {
        key_id        = "${user.name}/${key.name}"
        user_name     = user.name
        name          = key.name
        description   = key.description
        expires       = key.expires
        ip_allow_list = key.ip_allow_list
        ip_deny_list  = key.ip_deny_list
      }
    ]
  ])
  api_keys = { for key in local.api_key_list : key.key_id => key }

  # plantimestamp() is an RFC3339 string. api_key_limit is only compared when
  # api_key_max_lifetime_seconds is set. The placeholder keeps that comparison valid.
  api_key_limit = var.api_key_max_lifetime_seconds == null ? plantimestamp() : timeadd(plantimestamp(), "${var.api_key_max_lifetime_seconds}s")

  applied_control_ids = setunion(
    toset(keys(local.applied_settings)),
    length(var.certificates) > 0 ? toset(["tls_certificate"]) : toset([]),
    length(local.api_keys) > 0 ? toset(["api_key_expiry"]) : toset([]),
    length(var.users) > 0 || length(var.groups) > 0 ? toset(["admin_accounts"]) : toset([]),
  )
}

resource "vergeio_setting" "this" {
  for_each = local.applied_settings

  key   = each.value.key
  value = each.value.value
}

resource "vergeio_certificate" "this" {
  for_each = local.certificates

  type                    = each.value.type
  domain_name             = each.value.domain_name
  domain_list             = each.value.domain_list
  description             = each.value.description
  public_certificate      = each.value.public_certificate
  private_key_wo          = lookup(var.certificate_private_keys, each.key, null)
  private_key_wo_version  = each.value.private_key_wo_version
  chain                   = each.value.chain
  acme_server             = each.value.acme_server
  eab_kid                 = each.value.eab_kid
  eab_hmac_key_wo         = lookup(var.certificate_eab_hmac_keys, each.key, null)
  eab_hmac_key_wo_version = each.value.eab_hmac_key_wo_version
  key_type                = each.value.key_type
  rsa_key_size            = each.value.rsa_key_size
  contact                 = each.value.contact
  agree_tos               = each.value.agree_tos
}

resource "vergeio_group" "this" {
  for_each = local.groups

  name        = each.value.name
  description = each.value.description
  enabled     = each.value.enabled
}

resource "vergeio_permission" "this" {
  for_each = local.permissions

  group_id  = vergeio_group.this[each.value.group_name].id
  table     = each.value.table
  object_id = each.value.object_id
  list      = each.value.list
  read      = each.value.read
  create    = each.value.create
  modify    = each.value.modify
  delete    = each.value.delete
}

resource "vergeio_user" "this" {
  for_each = local.users

  name                = each.value.name
  displayname         = each.value.displayname
  email               = each.value.email
  enabled             = each.value.enabled
  type                = each.value.type
  change_password     = each.value.change_password
  password_wo         = lookup(var.passwords, each.key, null)
  password_wo_version = contains(nonsensitive(keys(var.passwords)), each.key) ? lookup(var.password_versions, each.key, 1) : null
}

resource "vergeio_member" "this" {
  for_each = local.memberships

  group  = tonumber(vergeio_group.this[each.value.group_name].id)
  member = format("users/%s", vergeio_user.this[each.value.user_name].id)
}

resource "vergeio_api_key" "this" {
  for_each = local.api_keys

  user_id       = tonumber(vergeio_user.this[each.value.user_name].id)
  name          = each.value.name
  description   = each.value.description
  expires       = each.value.expires
  ip_allow_list = each.value.ip_allow_list
  ip_deny_list  = each.value.ip_deny_list
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition = alltrue([
        for id in keys(var.settings) : contains(keys(local.setting_catalog), id)
      ])
      error_message = "settings keys must be control ids: login_lockout_attempts, login_lockout_minutes, session_expiration, session_inactivity, logon_banner_header, logon_banner_text, password_complexity."
    }

    precondition {
      condition = alltrue([
        for id in local.required_setting_ids : contains(keys(var.settings), id)
      ])
      error_message = "The strict profile writes every setting control. Add each control id to settings. Set apply_settings to false to leave system settings unchanged."
    }

    precondition {
      condition     = var.profile != "strict" || length(var.certificates) > 0
      error_message = "The strict profile includes a TLS certificate."
    }

    precondition {
      condition     = var.profile != "strict" || (length(var.users) > 0 && length(var.groups) > 0)
      error_message = "The strict profile includes a user and a group."
    }

    precondition {
      condition = var.profile != "strict" || alltrue([
        for user in var.users : length(user.groups) > 0
      ])
      error_message = "The strict profile puts each user in a group."
    }

    precondition {
      condition = alltrue([
        for user in var.users : contains(nonsensitive(keys(var.passwords)), user.name)
      ])
      error_message = "Each user needs a password in passwords, keyed by the user name."
    }

    precondition {
      condition = alltrue([
        for user in var.users : alltrue([
          for group_name in user.groups : contains([for group in var.groups : group.name], group_name)
        ])
      ])
      error_message = "users.groups must name a group from groups."
    }

    precondition {
      condition = alltrue([
        for key in local.api_key_list :
        timecmp(plantimestamp(), timeadd("1970-01-01T00:00:00Z", "${key.expires}s")) < 0 && (
          var.api_key_max_lifetime_seconds == null || timecmp(timeadd("1970-01-01T00:00:00Z", "${key.expires}s"), local.api_key_limit) <= 0
        )
      ])
      error_message = "Each API key expires must be in the future. When api_key_max_lifetime_seconds is set, expires must fall inside that window."
    }
  }
}
