output "profile" {
  description = "Selected profile name."
  value       = var.profile
}

output "controls" {
  description = "Control mapping for the selected profile. applied is true when this configuration writes that control. documented_value is the product-guide default for a setting that publishes one."
  value = merge(
    {
      for id, control in local.setting_catalog : id => {
        title            = control.title
        guide            = control.guide
        resource         = "vergeio_setting"
        documented_value = control.documented_value
        required         = var.profile == "strict" && control.strict
        applied          = contains(local.applied_control_ids, id)
      }
    },
    {
      for id, control in local.other_controls : id => {
        title            = control.title
        guide            = control.guide
        resource         = control.resource
        documented_value = control.documented_value
        required         = var.profile == "strict" && control.strict
        applied          = contains(local.applied_control_ids, id)
      }
    },
  )
}

output "setting_ids" {
  description = "vergeio_setting ids, keyed by control id. The id is the VergeOS key."
  value       = { for id, setting in vergeio_setting.this : id => setting.id }
}

output "setting_keys" {
  description = "VergeOS setting keys this configuration writes, keyed by control id."
  value       = { for id, setting in vergeio_setting.this : id => setting.key }
}

output "certificate_ids" {
  description = "Certificate keys, keyed by certificate name."
  value       = { for name, cert in vergeio_certificate.this : name => cert.id }
}

output "group_ids" {
  description = "Group keys, keyed by group name."
  value       = { for name, group in vergeio_group.this : name => group.id }
}

output "user_ids" {
  description = "User keys, keyed by user name."
  value       = { for name, user in vergeio_user.this : name => user.id }
}

output "member_ids" {
  description = "Membership keys, keyed by user name and group name."
  value       = { for key, member in vergeio_member.this : key => member.id }
}

output "permission_ids" {
  description = "Permission keys, keyed by group, table, and object."
  value       = { for key, perm in vergeio_permission.this : key => perm.id }
}

output "api_key_ids" {
  description = "API key ids, keyed by user name and key name. The bearer token is not stored."
  value       = { for key, api_key in vergeio_api_key.this : key => api_key.id }
}
