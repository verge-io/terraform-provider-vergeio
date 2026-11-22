# Lab apply test. The module's terraform test does not load this directory.
# Run it against a lab with this provider build:
#   terraform -chdir=tests/lab init -backend=false
#   terraform -chdir=tests/lab test -parallelism=1
#
# Every VergeOS object is named tf-acc-k8s or tf-acc-k8s-. The run creates
# the network, the nodes, the pool VM, the API user, and the grants.
# Destroy removes those objects.

provider "vergeio" {}

run "apply" {
  command = apply

  assert {
    condition     = output.network_id != null && output.network_id != "" && output.endpoint == "https://192.168.100.10:6443" && length(output.control_plane_ids) == 1 && length(output.worker_ids) == 1 && output.pool_vm_id != null && output.pool_vm_id != "" && output.node_ips["cp-1"] == "192.168.100.10" && output.node_ips["worker-1"] == "192.168.100.40" && output.vergeos_api_key != ""
    error_message = "Lab apply did not store the tf-acc-k8s network, nodes, pool VM, and API token."
  }
}
