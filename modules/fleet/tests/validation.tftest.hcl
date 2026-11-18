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
      powerstate   = false
      dhcp_enabled = true
      firewall_rules = [{
        name     = "allow-ssh"
        protocol = "tcp"
      }]
    }
  }

  tenants = {
    canary = {
      wave = "canary"
      name = "customer-canary"
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
        name = "canary-admin"
      }
      networks = {
        app = {
          cidr       = "10.83.10.0/24"
          dhcp_start = "10.83.10.10"
          dhcp_stop  = "10.83.10.50"
        }
      }
    }
    later = {
      wave = "rest"
      name = "customer-later"
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
    name = "tenant-admins"
  }
}

run "empty_waves" {
  command = plan

  variables {
    waves = []
  }

  expect_failures = [
    var.waves,
  ]
}

run "unknown_wave" {
  command = plan

  variables {
    waves = ["missing"]
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "password_needs_digit" {
  command = plan

  variables {
    tenant_passwords = {
      canary = "Fleet-password"
      later  = "Fleet-password2"
    }
  }

  expect_failures = [
    var.tenant_passwords,
  ]
}

run "later_password_missing" {
  command = plan

  variables {
    tenant_passwords = {
      canary = "Fleet-password1"
    }
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "duplicate_names" {
  command = plan

  variables {
    tenants = {
      canary = {
        wave = "canary"
        name = "customer"
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
          name = "canary-admin"
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            dhcp_start = "10.83.10.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
      later = {
        wave = "rest"
        name = "customer"
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
  }

  expect_failures = [
    var.tenants,
  ]
}

run "slash_in_key" {
  command = plan

  variables {
    tenants = {
      "canary/east" = {
        wave = "canary"
        name = "customer-canary"
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
          name = "canary-admin"
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            dhcp_start = "10.83.10.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
    }
  }

  expect_failures = [
    var.tenants,
  ]
}

run "missing_network" {
  command = plan

  variables {
    tenants = {
      canary = {
        wave = "canary"
        name = "customer-canary"
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
          name = "canary-admin"
        }
        networks = {}
      }
      later = {
        wave = "rest"
        name = "customer-later"
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
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "duplicate_cidr" {
  command = plan

  variables {
    tenants = {
      canary = {
        wave = "canary"
        name = "customer-canary"
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
          name = "canary-admin"
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            dhcp_start = "10.83.10.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
      later = {
        wave = "rest"
        name = "customer-later"
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
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            dhcp_start = "10.83.10.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
    }
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "dhcp_outside_cidr" {
  command = plan

  variables {
    tenants = {
      canary = {
        wave = "canary"
        name = "customer-canary"
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
          name = "canary-admin"
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            dhcp_start = "10.9.9.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
      later = {
        wave = "rest"
        name = "customer-later"
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
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "prefixed_ip" {
  command = plan

  variables {
    tenants = {
      canary = {
        wave = "canary"
        name = "customer-canary"
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
          name = "canary-admin"
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            ipaddress  = "10.83.10.1/24"
            dhcp_start = "10.83.10.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
    }
  }

  expect_failures = [
    var.tenants,
  ]
}

run "storage_bytes" {
  command = plan

  variables {
    tenants = {
      canary = {
        wave = "canary"
        name = "customer-canary"
        nodes = [{
          name      = "canary-node1"
          cpu_cores = 2
          ram       = 2048
        }]
        storage = [{
          tier        = 1
          provisioned = 1000
        }]
        admin_user = {
          name = "canary-admin"
        }
        networks = {
          app = {
            cidr       = "10.83.10.0/24"
            dhcp_start = "10.83.10.10"
            dhcp_stop  = "10.83.10.50"
          }
        }
      }
    }
  }

  expect_failures = [
    var.tenants,
  ]
}

run "user_collision" {
  command = plan

  variables {
    baseline_users = [{
      name   = "canary-admin"
      groups = []
    }]
    baseline_passwords = {
      canary-admin = "Fleet-password1"
    }
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "group_collision" {
  command = plan

  variables {
    baseline_groups = [{
      name = "tenant-admins"
    }]
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "version_key" {
  command = plan

  variables {
    tenant_password_versions = {
      missing = 2
    }
  }

  expect_failures = [
    terraform_data.check,
  ]
}
