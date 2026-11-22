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

run "tenant_without_snapshot" {
  command = plan

  variables {
    source_type = "tenant"
    tenant_id   = "7"
    tag         = null
    cidr        = null
    tenant_snapshots = [{
      id      = "3"
      name    = "nightly"
      created = 10
    }]
    snapshot_name = "missing"
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
