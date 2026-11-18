mock_provider "vergeio" {
  mock_resource "vergeio_group" {
    defaults = {
      id = "1"
    }
  }
}

variables {
  system_key = "lab"
  waves      = ["canary"]

  networks = {
    app = {
      description     = "Application segment"
      enabled         = true
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

  tenants = {
    canary = {
      wave        = "canary"
      name        = "customer-canary"
      description = "Canary tenant"
      powerstate  = false
      nodes = [{
        name      = "canary-node1"
        cpu_cores = 2
        ram       = 2048
      }]
      storage = [{
        tier        = 1
        provisioned = 1073741824
      }]
      admin_user = {
        name        = "canary-admin"
        displayname = "Canary admin"
        type        = "normal"
      }
      networks = {
        app = {
          cidr       = "10.83.10.0/24"
          dhcp_start = "10.83.10.10"
          dhcp_stop  = "10.83.10.50"
          domain     = "canary.example.com"
          dns = {
            zone_domain = "canary.example.com"
            email       = "hostmaster@example.com"
            records = [{
              host  = "www"
              type  = "A"
              value = "10.83.10.10"
            }]
          }
        }
      }
    }
    later = {
      wave        = "rest"
      name        = "customer-later"
      description = "Later tenant"
      powerstate  = false
      nodes = [{
        name      = "later-node1"
        cpu_cores = 2
        ram       = 2048
      }]
      storage = [{
        tier        = 1
        provisioned = 1073741824
      }]
      admin_user = {
        name = "later-admin"
        type = "normal"
      }
      networks = {
        app = {
          cidr       = "10.83.11.0/24"
          dhcp_start = "10.83.11.10"
          dhcp_stop  = "10.83.11.50"
        }
      }
    }
  }

  tenant_passwords = {
    canary = "Fleet-password1"
    later  = "Fleet-password2"
  }

  admin_passwords = {
    canary = "Fleet-admin1"
    later  = "Fleet-admin2"
  }

  tenant_admin_group = {
    name        = "tenant-admins"
    description = "Tenant admins"
    enabled     = true
    permissions = [{
      table  = "tenants"
      list   = true
      read   = true
      create = false
      modify = false
      delete = false
    }]
  }

  baseline_profile        = "baseline"
  baseline_apply_settings = true
  baseline_settings = {
    login_lockout_attempts = {
      key   = "operator-login-lockout-attempts"
      value = "0"
    }
  }
}

run "canary" {
  command = plan

  assert {
    condition     = output.system_key == "lab" && output.waves == toset(["canary"]) && output.pending_waves == toset(["rest"])
    error_message = "The canary wave was not the only wave planned."
  }

  assert {
    condition     = output.tenant_keys == toset(["canary"]) && output.pending_tenant_keys == toset(["later"]) && output.tenant_names["canary"] == "customer-canary"
    error_message = "The canary tenant was not planned on its own."
  }

  assert {
    condition     = output.network_names["canary/app"] == "customer-canary-app" && output.network_cidrs["canary/app"] == "10.83.10.0/24" && output.network_addresses["canary/app"] == "10.83.10.1" && !strcontains(output.network_addresses["canary/app"], "/")
    error_message = "The canary network was not planned with a bare router address from the CIDR."
  }

  assert {
    condition     = output.firewall_rule_names["canary/app"] == ["allow-ssh"] && !contains(keys(output.network_cidrs), "later/app")
    error_message = "The shared firewall policy was not planned for the canary only."
  }

  assert {
    condition     = vergeio_group.tenant_admins.name == "tenant-admins" && vergeio_group.tenant_admins.enabled == true && vergeio_permission.tenant_admins["tenants/table"].table == "tenants" && vergeio_permission.tenant_admins["tenants/table"].read == true && vergeio_permission.tenant_admins["tenants/table"].delete == false
    error_message = "The tenant admin group was not planned."
  }

  assert {
    condition     = output.baseline_setting_keys["login_lockout_attempts"] == "operator-login-lockout-attempts" && output.baseline_controls["login_lockout_attempts"].applied == true && output.baseline_profile == "baseline"
    error_message = "The baseline setting was not planned from the caller's key."
  }
}

run "rest" {
  command = plan

  variables {
    waves = ["canary", "rest"]
  }

  assert {
    condition     = output.tenant_keys == toset(["canary", "later"]) && length(output.pending_tenant_keys) == 0 && length(output.pending_waves) == 0
    error_message = "Adding the rest wave did not keep the canary and add the later tenant."
  }

  assert {
    condition     = output.network_cidrs["canary/app"] == "10.83.10.0/24" && output.network_cidrs["later/app"] == "10.83.11.0/24" && output.network_addresses["later/app"] == "10.83.11.1" && output.network_names["later/app"] == "customer-later-app"
    error_message = "The rest wave did not plan a separate bare address for the later tenant."
  }
}

run "settings_unchanged" {
  command = plan

  variables {
    baseline_apply_settings = false
  }

  assert {
    condition     = length(output.baseline_setting_keys) == 0 && output.baseline_controls["login_lockout_attempts"].applied == false && output.tenant_names["canary"] == "customer-canary"
    error_message = "baseline_apply_settings false still planned a system setting."
  }
}
