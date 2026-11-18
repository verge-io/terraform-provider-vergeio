mock_provider "vergeio" {}

variables {
  name            = "web1"
  source_type     = "image"
  media_source_id = 25
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

run "image_requires_media" {
  command = plan

  variables {
    media_source_id = null
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "recipe_requires_key" {
  command = plan

  variables {
    source_type     = "recipe"
    recipe_id       = "not-a-key"
    media_source_id = null
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "recipe_has_no_snapshot_profile" {
  command = plan

  variables {
    source_type     = "recipe"
    recipe_id       = "0123456789abcdef0123456789abcdef01234567"
    media_source_id = null
    snapshot_profile = {
      name = "nightly"
      periods = [{
        name      = "nightly"
        frequency = "daily"
        retention = 604800
      }]
    }
  }

  expect_failures = [
    terraform_data.check,
  ]
}

run "snapshot_retention" {
  command = plan

  variables {
    snapshot_profile = {
      name = "nightly"
      periods = [{
        name      = "nightly"
        frequency = "daily"
        retention = 0
      }]
    }
  }

  expect_failures = [
    var.snapshot_profile,
  ]
}
