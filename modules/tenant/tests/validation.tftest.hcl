mock_provider "vergeio" {}

variables {
  name               = "customer-a"
  tenant_password_wo = "Tenant-admin-password1"
  admin_group_id     = 1
  admin_password_wo  = "Parent-admin-password1"

  admin_user = {
    name = "customer-a-admin"
  }

  nodes = [{
    name      = "node1"
    cpu_cores = 4
    ram       = 8192
  }]

  storage = [{
    tier        = 1
    provisioned = 107374182400
  }]

  network_blocks = [{
    network_id = "3"
    cidr       = "198.51.100.0/28"
  }]

  external_ips = [{
    network_id = "3"
    ip         = "203.0.113.50"
  }]
}

run "storage_must_be_whole_gib" {
  command = plan

  variables {
    storage = [{
      tier        = 1
      provisioned = 100
    }]
  }

  expect_failures = [
    var.storage,
  ]
}

run "node_ram_minimum" {
  command = plan

  variables {
    nodes = [{
      name      = "node1"
      cpu_cores = 4
      ram       = 1024
    }]
  }

  expect_failures = [
    var.nodes,
  ]
}

run "block_must_be_network_address" {
  command = plan

  variables {
    network_blocks = [{
      network_id = "3"
      cidr       = "198.51.100.1/28"
    }]
  }

  expect_failures = [
    var.network_blocks,
  ]
}

run "password_needs_uppercase" {
  command = plan

  variables {
    tenant_password_wo = "tenant-admin-password1"
  }

  expect_failures = [
    var.tenant_password_wo,
  ]
}

run "admin_user_type" {
  command = plan

  variables {
    admin_user = {
      name = "customer-a-admin"
      type = "service"
    }
  }

  expect_failures = [
    var.admin_user,
  ]
}
