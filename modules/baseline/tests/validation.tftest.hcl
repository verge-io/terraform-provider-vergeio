mock_provider "vergeio" {}

variables {
  passwords = {
    ops = "Baseline-password1"
  }

  groups = [{
    name = "operators"
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
    name   = "ops"
    groups = ["operators"]
  }]

  certificates = [{
    name        = "ui"
    type        = "self_signed"
    domain_name = "ui.example.com"
  }]
}

run "profile" {
  command = plan

  variables {
    profile = "custom"
  }

  expect_failures = [
    var.profile,
  ]
}

run "password_needs_uppercase" {
  command = plan

  variables {
    passwords = {
      ops = "baseline-password1"
    }
  }

  expect_failures = [
    var.passwords,
  ]
}

run "user_type" {
  command = plan

  variables {
    users = [{
      name   = "ops"
      type   = "service"
      groups = ["operators"]
    }]
  }

  expect_failures = [
    var.users,
  ]
}

run "setting_key_spaces" {
  command = plan

  variables {
    settings = {
      login_lockout_attempts = {
        key   = "bad key"
        value = "0"
      }
    }
  }

  expect_failures = [
    var.settings,
  ]
}

run "duplicate_setting_keys" {
  command = plan

  variables {
    settings = {
      login_lockout_attempts = {
        key   = "same-key"
        value = "0"
      }
      login_lockout_minutes = {
        key   = "same-key"
        value = "15"
      }
    }
  }

  expect_failures = [
    var.settings,
  ]
}

run "unknown_control" {
  command = plan

  variables {
    settings = {
      not_a_control = {
        key   = "max_connections"
        value = "500"
      }
    }
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "read_requires_list" {
  command = plan

  variables {
    groups = [{
      name = "operators"
      permissions = [{
        table = "vms"
        list  = false
        read  = true
      }]
    }]
  }

  expect_failures = [
    var.groups,
  ]
}

run "self_signed_needs_domain" {
  command = plan

  variables {
    certificates = [{
      name = "ui"
      type = "self_signed"
    }]
  }

  expect_failures = [
    var.certificates,
  ]
}

run "manual_needs_public_certificate" {
  command = plan

  variables {
    certificates = [{
      name                   = "ui"
      type                   = "manual"
      private_key_wo_version = 1
    }]
  }

  expect_failures = [
    var.certificates,
  ]
}

run "missing_group" {
  command = plan

  variables {
    users = [{
      name   = "ops"
      groups = ["missing"]
    }]
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "expired_api_key" {
  command = plan

  variables {
    users = [{
      name   = "ops"
      groups = ["operators"]
      api_keys = [{
        name    = "ci"
        expires = 1
      }]
    }]
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "api_key_lifetime" {
  command = plan

  variables {
    api_key_max_lifetime_seconds = 3600

    users = [{
      name   = "ops"
      groups = ["operators"]
      api_keys = [{
        name    = "ci"
        expires = 1893456000
      }]
    }]
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "strict_requires_settings" {
  command = plan

  variables {
    profile = "strict"
  }

  expect_failures = [
    terraform_data.check,
  ]
}
