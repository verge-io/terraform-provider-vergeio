# Lab apply test. The module's terraform test does not load this directory.
# apply_settings is false in tests/lab/main.tf, so this run does not write
# system settings. Run it against a lab with:
#   terraform -chdir=tests/lab init -backend=false
#   terraform -chdir=tests/lab test

provider "vergeio" {}

run "apply" {
  command = apply

  assert {
    condition     = output.user_ids["tf-acc-baseline-admin"] != "" && output.user_ids["tf-acc-baseline-operator"] != "" && output.group_ids["tf-acc-baseline-admins"] != "" && output.certificate_ids["tf-acc-baseline"] != "" && output.api_key_ids["tf-acc-baseline-admin/tf-acc-baseline-ci"] != "" && length(output.setting_ids) == 0
    error_message = "Lab apply did not store the tf-acc-baseline user, group, certificate, and API key, or it wrote a system setting."
  }
}
