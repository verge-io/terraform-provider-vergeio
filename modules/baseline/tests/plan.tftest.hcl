mock_provider "vergeio" {}

# The key strings below stand in for keys an operator reads from VergeOS.
# The provider does not publish those stored names. This test checks wiring.

variables {
  profile        = "baseline"
  apply_settings = true

  settings = {
    login_lockout_attempts = {
      key   = "operator-login-lockout-attempts"
      value = "0"
    }
    login_lockout_minutes = {
      key   = "operator-login-lockout-minutes"
      value = "15"
    }
    session_expiration = {
      key   = "operator-session-expiration"
      value = "259200"
    }
    session_inactivity = {
      key   = "operator-session-inactivity"
      value = "86400"
    }
    logon_banner_header = {
      key   = "operator-logon-banner-header"
      value = "Authorized use"
    }
    logon_banner_text = {
      key   = "operator-logon-banner-text"
      value = "Authorized use only."
    }
    password_complexity = {
      key   = "operator-password-complexity"
      value = "length:8"
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
    enabled         = true
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
    ops = "Baseline-password1"
  }
}

run "plan" {
  command = plan

  assert {
    condition     = vergeio_setting.this["login_lockout_attempts"].key == "operator-login-lockout-attempts" && vergeio_setting.this["login_lockout_attempts"].value == "0"
    error_message = "Login lockout was not planned from the caller's key and value."
  }

  assert {
    condition     = vergeio_setting.this["session_expiration"].value == "259200" && vergeio_setting.this["session_inactivity"].value == "86400" && vergeio_setting.this["password_complexity"].value == "length:8"
    error_message = "Session and password settings were not planned from the caller's values."
  }

  assert {
    condition     = vergeio_setting.this["logon_banner_text"].value == "Authorized use only." && length(vergeio_setting.this) == 7
    error_message = "The logon banner was not planned with the other setting controls."
  }

  assert {
    condition     = output.controls["login_lockout_attempts"].guide == "Failed login attempts before an account is locked (0 disables lockout)" && output.controls["login_lockout_attempts"].documented_value == "0" && output.controls["login_lockout_attempts"].applied == true
    error_message = "The login lockout control mapping was not planned."
  }

  assert {
    condition     = output.controls["logon_banner_text"].documented_value == null && output.profile == "baseline" && output.controls["api_key_expiry"].required == false
    error_message = "The banner default and profile were not planned."
  }

  assert {
    condition     = vergeio_certificate.this["ui"].type == "self_signed" && vergeio_certificate.this["ui"].domain_name == "ui.example.com" && vergeio_certificate.this["ui"].key_type == "ecdsa"
    error_message = "The TLS certificate was not planned."
  }

  assert {
    condition     = vergeio_group.this["operators"].name == "operators" && vergeio_group.this["operators"].enabled == true && vergeio_permission.this["operators/vms/table"].table == "vms" && vergeio_permission.this["operators/vms/table"].read == true && vergeio_permission.this["operators/vms/table"].delete == false
    error_message = "The operator group grant was not planned."
  }

  assert {
    condition     = vergeio_user.this["ops"].name == "ops" && vergeio_user.this["ops"].type == "normal" && vergeio_user.this["ops"].change_password == true && length(vergeio_member.this) == 1
    error_message = "The admin user membership was not planned."
  }

  assert {
    condition     = vergeio_api_key.this["ops/ci"].name == "ci" && vergeio_api_key.this["ops/ci"].expires == 1893456000 && vergeio_api_key.this["ops/ci"].ip_allow_list == "192.0.2.10"
    error_message = "The API key expiry was not planned."
  }
}

run "strict" {
  command = plan

  variables {
    profile = "strict"
  }

  assert {
    condition     = output.profile == "strict" && output.controls["session_expiration"].required == true && output.controls["tls_certificate"].applied == true && output.controls["admin_accounts"].applied == true
    error_message = "The strict profile did not mark the setting, certificate, and account controls."
  }
}

run "manual_certificate" {
  command = plan

  variables {
    certificates = [{
      name                   = "uploaded"
      type                   = "manual"
      domain_name            = "ui.example.com"
      description            = "Uploaded UI certificate"
      public_certificate     = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
      private_key_wo_version = 1
    }]

    certificate_private_keys = {
      uploaded = "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"
    }
  }

  assert {
    condition     = length(vergeio_certificate.this) == 1 && vergeio_certificate.this["uploaded"].type == "manual" && vergeio_certificate.this["uploaded"].private_key_wo_version == 1 && vergeio_certificate.this["uploaded"].domain_name == "ui.example.com"
    error_message = "The manual certificate was not planned."
  }
}

run "skip_settings" {
  command = plan

  variables {
    apply_settings = false
  }

  assert {
    condition     = length(vergeio_setting.this) == 0 && output.controls["login_lockout_attempts"].applied == false && length(vergeio_user.this) == 1 && length(vergeio_certificate.this) == 1
    error_message = "apply_settings false still planned a system setting, or it dropped the user and certificate."
  }
}
