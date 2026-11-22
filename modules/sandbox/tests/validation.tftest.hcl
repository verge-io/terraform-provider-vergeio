mock_provider "vergeio" {}

variables {
  name        = "pr-128"
  source_type = "vm"
  tag         = "app/billing"
  cidr        = "192.168.91.0/24"
}

run "source_type" {
  command = plan

  variables {
    source_type = "disk"
  }

  expect_failures = [
    var.source_type,
  ]
}

run "uplink_without_opt_in" {
  command = plan

  variables {
    external_network_id = 3
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "external_without_network" {
  command = plan

  variables {
    allow_external = true
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "tenant_snapshot_name" {
  command = plan

  variables {
    source_type   = "tenant"
    tenant_id     = "7"
    tag           = null
    cidr          = null
    snapshot_name = "nightly"
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "vm_without_tag" {
  command = plan

  variables {
    tag = null
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "vms_empty" {
  command = plan

  variables {
    tag = null
    vms = []
  }

  assert {
    condition     = length(vergeio_network.sandbox) == 0 && length(vergeio_vm_clone.this) == 0
    error_message = "An empty vms list planned a sandbox network or a clone."
  }
}

run "tag_matches_nothing" {
  command = plan

  variables {
    tag = "app/missing"
    vms = null
  }

  assert {
    condition     = length(vergeio_network.sandbox) == 0 && length(vergeio_vm_clone.this) == 0
    error_message = "A tag that matches no VMs planned a sandbox network or a clone."
  }
}
