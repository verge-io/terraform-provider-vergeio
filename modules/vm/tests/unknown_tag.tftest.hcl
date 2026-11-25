mock_provider "vergeio" {}

run "tag_id_unknown_at_plan" {
  command = plan

  module {
    source = "./tests/unknown_tag"
  }

  assert {
    condition     = output.tag_member_keys == ["web"]
    error_message = "A tag id that is unknown at plan time could not be used as a membership."
  }
}
