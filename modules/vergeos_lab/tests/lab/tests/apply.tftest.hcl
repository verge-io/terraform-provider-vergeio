# Lab apply test. The module's terraform test does not load this directory.
# It creates only tf-acc- objects, writes no system settings, and does not
# read an existing vnet, media source, tenant, user, or setting.
# Run it against a lab with one operation at a time. The lab API drops
# connections under concurrent writes.
#   terraform -chdir=tests/lab init -backend=false
#   terraform -chdir=tests/lab test -parallelism=1

provider "vergeio" {}

run "apply" {
  command = apply

  assert {
    condition     = output.vm_ids["controller-1"] != "" && output.vm_ids["controller-2"] != "" && length(output.vm_ids) == 2 && output.vm_names["controller-1"] == "tf-acc-vergeos-lab-controller-1" && output.vm_names["controller-2"] == "tf-acc-vergeos-lab-controller-2" && output.core_network_ids.core_1 != "" && output.core_network_ids.core_2 != "" && output.external_network_id != "" && output.vergeos_version == "26.0.0"
    error_message = "Lab apply did not store the tf-acc-vergeos-lab controllers, fabric networks, and external network."
  }
}
