mock_provider "vergeio" {}

variables {
  name           = "edge"
  source_type    = "image"
  cidr           = "192.168.100.0/24"
  vergeos_host   = "https://vergeos.example.com"
  boot_disk_size = 8
}

run "source_type" {
  command = plan

  variables {
    source_type = "clone"
  }

  expect_failures = [
    var.source_type,
  ]
}

run "image_without_disk" {
  command = plan

  variables {
    boot_disk_size = null
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "recipe_key" {
  command = plan

  variables {
    source_type    = "recipe"
    boot_disk_size = null
    recipe_id      = "abcd"
  }

  expect_failures = [
    var.recipe_id,
  ]
}

run "recipe_media" {
  command = plan

  variables {
    source_type     = "recipe"
    boot_disk_size  = null
    media_source_id = 4
    recipe_id       = "0123456789abcdef0123456789abcdef01234567"
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "cidr" {
  command = plan

  variables {
    cidr = "192.168.100.0/25"
  }

  expect_failures = [
    var.cidr,
  ]
}

run "name" {
  command = plan

  variables {
    name = "Edge"
  }

  expect_failures = [
    var.name,
  ]
}

run "counts" {
  command = plan

  variables {
    control_plane_count = 0
  }

  expect_failures = [
    var.control_plane_count,
  ]
}
