mock_provider "vergeio" {}

run "password_unknown_at_plan" {
  command = plan

  module {
    source = "./tests/unknown_password"
  }

  assert {
    condition     = output.vm_names["controller-1"] == "lab1-controller-1" && output.vm_names["controller-2"] == "lab1-controller-2"
    error_message = "A password and network key that are unknown at plan time could not plan the lab."
  }
}
