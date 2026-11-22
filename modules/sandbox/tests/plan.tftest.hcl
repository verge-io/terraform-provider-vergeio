mock_provider "vergeio" {}

variables {
  name        = "pr-128"
  source_type = "vm"
  cidr        = "192.168.91.0/24"
  vms = [
    {
      key  = 15
      name = "api"
      snapshots = [
        {
          id      = "2"
          name    = "older"
          created = 10
        },
        {
          id      = "9"
          name    = "newer"
          created = 30
        },
      ]
    },
    {
      key  = 16
      name = "old-snap"
      snapshots = [
        {
          id      = "4"
          name    = "snap"
          created = 1
        },
      ]
    },
  ]
}

run "vms" {
  command = plan

  assert {
    condition     = vergeio_network.sandbox[0].name == "pr-128" && vergeio_network.sandbox[0].type == "internal" && vergeio_network.sandbox[0].network == "192.168.91.0/24" && vergeio_network.sandbox[0].layer2_type == "none" && vergeio_network.sandbox[0].interface_vnet == 0 && vergeio_network.sandbox[0].dhcp_enabled == false && vergeio_network.sandbox[0].powerstate == false && vergeio_network.sandbox[0].vnet_default_gateway == null
    error_message = "The sandbox network was planned with a route out."
  }

  assert {
    condition     = vergeio_vm_clone.this["15"].snapshot_id == "9" && vergeio_vm_clone.this["15"].name == "pr-128-api" && vergeio_vm_clone.this["16"].snapshot_id == "4" && length(vergeio_vm_clone.this) == 2 && length(vergeio_tenant_copy.this) == 0 && length(data.vergeio_vms.tagged) == 0 && output.isolated == true
    error_message = "The newest snapshot of each VM was not cloned onto the sandbox network."
  }
}

run "named_snapshot_and_uplink" {
  command = plan

  variables {
    snapshot_name       = "older"
    allow_external      = true
    external_network_id = 3
    vms = [
      {
        key  = 15
        name = "api"
        snapshots = [
          {
            id      = "2"
            name    = "older"
            created = 10
          },
          {
            id      = "9"
            name    = "newer"
            created = 30
          },
        ]
      },
    ]
  }

  assert {
    condition     = vergeio_vm_clone.this["15"].snapshot_id == "2" && vergeio_network.sandbox[0].vnet_default_gateway == 3 && vergeio_network.sandbox[0].powerstate == true && output.isolated == false
    error_message = "The named snapshot was not cloned, or the uplink was not planned."
  }
}

run "tenant" {
  command = plan

  variables {
    source_type = "tenant"
    tenant_id   = "7"
    cidr        = null
    vms         = null
  }

  assert {
    condition     = length(vergeio_tenant_copy.this) == 1 && vergeio_tenant_copy.this[0].tenant_id == "7" && vergeio_tenant_copy.this[0].name == "pr-128" && vergeio_tenant_copy.this[0].isolate == true && length(vergeio_network.sandbox) == 0 && length(vergeio_vm_clone.this) == 0
    error_message = "The tenant copy was not planned from the current tenant, or a sandbox network was planned."
  }
}
