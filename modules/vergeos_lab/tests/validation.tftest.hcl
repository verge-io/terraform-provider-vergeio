mock_provider "vergeio" {}

variables {
  name                = "lab1"
  vergeos_version     = "26.0.2"
  admin_password      = "Labpass1"
  admin_email         = "admin@example.com"
  external_network_id = 6
  external_ip         = "198.51.100.10"
  external_prefix     = 24
  external_gateway    = "198.51.100.1"
  external_dns        = "198.51.100.1"
}

run "shape" {
  command = plan

  variables {
    shape = "cluster"
  }

  expect_failures = [
    var.shape,
  ]
}

run "version" {
  command = plan

  variables {
    vergeos_version = "latest"
  }

  expect_failures = [
    var.vergeos_version,
  ]
}

run "password" {
  command = plan

  variables {
    admin_password = "labpass1"
  }

  expect_failures = [
    var.admin_password,
  ]
}

run "external_ip_prefix" {
  command = plan

  variables {
    external_ip = "198.51.100.10/24"
  }

  expect_failures = [
    var.external_ip,
  ]
}

run "fabric_host_bits" {
  command = plan

  variables {
    core_fabric_1_cidr = "172.16.1.1/24"
  }

  expect_failures = [
    var.core_fabric_1_cidr,
  ]
}

run "fabric_cidrs_must_differ" {
  command = plan

  variables {
    core_fabric_2_cidr = "172.16.1.0/24"
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "scale_out_count" {
  command = plan

  variables {
    shape           = "scale_out"
    scale_out_count = 0
  }

  expect_failures = [
    var.scale_out_count,
  ]
}

run "timeout_while_off" {
  command = plan

  variables {
    wait_for_guest_ip_timeout = 30
  }

  expect_failures = [
    terraform_data.check,
  ]
}
