mock_provider "vergeio" {}

variables {
  name                       = "customer-a"
  description                = "Customer A virtual data center"
  tenant_password_wo         = "Tenant-admin-password1"
  tenant_password_wo_version = 1
  change_password            = true
  powerstate                 = true
  isolate                    = false
  admin_group_id             = 1
  admin_password_wo          = "Parent-admin-password1"
  admin_password_wo_version  = 1

  admin_user = {
    name            = "customer-a-admin"
    displayname     = "Customer A admin"
    email           = "customer-a-admin@example.com"
    type            = "normal"
    enabled         = true
    change_password = true
  }

  nodes = [
    {
      name          = "node1"
      description   = "Primary node"
      cpu_cores     = 4
      ram           = 8192
      enabled       = true
      on_power_loss = "last_state"
    },
    {
      name      = "node2"
      cpu_cores = 2
      ram       = 4096
    },
  ]

  storage = [{
    tier        = 1
    provisioned = 107374182400
  }]

  network_blocks = [{
    network_id            = "3"
    cidr                  = "198.51.100.0/28"
    description           = "Customer routed addresses"
    apply_parent_firewall = true
  }]

  external_ips = [{
    network_id            = "3"
    ip                    = "203.0.113.50"
    hostname              = "customer-a"
    description           = "Tenant UI address"
    apply_parent_firewall = true
  }]
}

run "plan" {
  command = plan

  assert {
    condition     = vergeio_tenant.this.name == "customer-a" && vergeio_tenant.this.powerstate == true && vergeio_tenant.this.isolate == false
    error_message = "Tenant name, power, and isolation were not planned from the module inputs."
  }

  assert {
    condition     = vergeio_tenant_node.this["node1"].cpu_cores == 4 && vergeio_tenant_node.this["node1"].ram == 8192 && vergeio_tenant_node.this["node2"].ram == 4096
    error_message = "Tenant nodes were not planned with the given CPU and RAM."
  }

  assert {
    condition     = vergeio_tenant_storage.this["1"].tier == 1 && vergeio_tenant_storage.this["1"].provisioned == 107374182400
    error_message = "Tenant storage was not planned as 100 GiB on tier 1."
  }

  assert {
    condition     = vergeio_tenant_network_block.this["198.51.100.0/28"].cidr == "198.51.100.0/28" && vergeio_tenant_network_block.this["198.51.100.0/28"].network_id == "3"
    error_message = "The routed network block was not planned."
  }

  assert {
    condition     = vergeio_tenant_external_ip.this["203.0.113.50"].ip == "203.0.113.50" && vergeio_tenant_external_ip.this["203.0.113.50"].hostname == "customer-a"
    error_message = "The external IP was not planned."
  }

  assert {
    condition     = vergeio_user.admin.name == "customer-a-admin" && vergeio_user.admin.type == "normal" && vergeio_member.admin.group == 1
    error_message = "The admin user was not planned as a member of the given group."
  }
}
