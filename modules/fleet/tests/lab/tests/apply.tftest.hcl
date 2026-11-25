# Lab apply test. The module's terraform test does not load this directory.
# baseline_apply_settings is false in tests/lab/main.tf, so this run does not
# write system settings. waves is the canary, so the later tenant is not
# created. Run it against a lab with:
#   terraform -chdir=tests/lab init -backend=false
#   terraform -chdir=tests/lab test

provider "vergeio" {}

run "apply" {
  command = apply

  assert {
    condition     = output.tenant_ids["canary"] != "" && length(output.tenant_ids) == 1 && output.pending_tenant_keys == toset(["later"])
    error_message = "Lab apply did not store only the tf-acc-fleet-canary tenant."
  }

  assert {
    condition     = output.network_ids["canary/app"] != "" && output.network_names["canary/app"] == "tf-acc-fleet-canary-app" && output.network_addresses["canary/app"] == "10.83.10.1" && !strcontains(output.network_addresses["canary/app"], "/")
    error_message = "Lab apply did not store the tf-acc-fleet-canary network with a bare router address."
  }

  assert {
    condition     = output.tenant_admin_group_id != "" && output.baseline_user_ids["tf-acc-fleet-operator"] != "" && output.baseline_certificate_ids["tf-acc-fleet"] != "" && output.baseline_api_key_ids["tf-acc-fleet-operator/tf-acc-fleet-ci"] != "" && length(output.baseline_setting_ids) == 0
    error_message = "Lab apply did not store the tf-acc-fleet group, user, certificate, and API key, or it wrote a system setting."
  }
}
