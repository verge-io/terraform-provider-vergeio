# Lab apply test. The module's terraform test does not load this directory.
# Run it against a lab with:
#   terraform -chdir=tests/lab init -backend=false
#   terraform -chdir=tests/lab test
#
# Every object this file creates is named tf-acc-. The first apply creates
# the throwaway sources and their snapshots. The second apply builds the
# sandboxes. Destroy removes the copies and the sources.

provider "vergeio" {}

run "prepare" {
  command = apply

  variables {
    enable_sandbox = false
  }
}

run "apply" {
  command = apply

  variables {
    enable_sandbox = true
  }

  assert {
    condition     = output.network_id != null && length(output.vm_ids) == 1 && output.tenant_id != null && output.isolated == true
    error_message = "Lab apply did not store the tf-acc-sandbox network, VM copy, and tenant copy."
  }
}
