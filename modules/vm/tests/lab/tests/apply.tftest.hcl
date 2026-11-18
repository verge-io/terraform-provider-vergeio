# Lab apply test. The module's terraform test does not load this directory.
# Run it against a lab with:
#   terraform -chdir=tests/lab init -backend=false
#   terraform -chdir=tests/lab test

provider "vergeio" {}

run "apply" {
  command = apply

  assert {
    condition     = output.vm_id != "" && output.network_id != ""
    error_message = "Lab apply did not store the tf-acc-vm id and its network id."
  }
}
