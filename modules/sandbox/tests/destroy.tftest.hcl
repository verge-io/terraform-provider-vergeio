mock_provider "vergeio" {}

variables {
  name        = "pr-128"
  source_type = "vm"
  tag         = null
  cidr        = "192.168.91.0/24"
  vms = [
    {
      key  = 15
      name = "api"
      snapshots = [
        {
          id      = "9"
          name    = "newer"
          created = 30
        },
      ]
    },
  ]
}

run "create" {
  command = apply

  override_resource {
    target = vergeio_network.sandbox[0]
    values = {
      id = "6"
    }
  }

  override_resource {
    target = vergeio_vm_clone.this["15"]
    values = {
      id         = "81"
      network_id = "6"
    }
  }

  assert {
    condition     = length(vergeio_network.sandbox) == 1 && length(vergeio_vm_clone.this) == 1
    error_message = "The sandbox network and clone were not created."
  }
}

run "destroy_empty_selection" {
  command = apply

  variables {
    vms = []
  }

  assert {
    condition     = length(vergeio_network.sandbox) == 0 && length(vergeio_vm_clone.this) == 0
    error_message = "An empty selection did not remove the sandbox network and clones."
  }
}
