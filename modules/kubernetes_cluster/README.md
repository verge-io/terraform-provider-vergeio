# kubernetes_cluster

Creates the VergeOS side of a Kubernetes cluster: the node VMs, an internal network, firewall rules, a stopped pool VM for the CSI block driver, and the API user the CSI driver and cloud controller use.

`source_type` `image` creates each node with `vergeio_vm`. Set `media_source_id` to import a media image. Omit it and set `boot_disk_size` for a blank disk. Cloud-init is `user-data` and `meta-data` on that VM. `user-data` is `contents_wo`, so the certificate private keys are not stored on the VM. Control planes receive `/etc/kubernetes/vergeio/ca.crt`, `ca.key`, `server.crt`, `server.key`, `admin.crt`, `admin.key`, and `kubeconfig`. Workers receive `ca.crt`. The hostname is the VM name. The cloud controller matches a node to a VM by that name when the provider ID is not `vergeos://<vm key>`.

`source_type` `recipe` creates each node with `vergeio_vm_recipe_instance`. `recipe_id` is the 40-character key from `vergeio_vm_recipes`. This module does not choose recipe question names. Put hostname, disk, and network answers in `recipe_answers`. That resource has no cloud-init attribute, so the certificates are not written to recipe nodes. No Kubernetes recipe is published in the verge-io GitHub org. The question names belong to the catalog on this system.

The network is `type` `internal`. Host 1 is the router. Hosts 10 and up are control planes, one address each. Hosts 40 and up are workers. Hosts 150 through 189 are the DHCP pool. Hosts 200 and up are left for the cloud controller load balancer pool and are not assigned to a NIC. DHCP is on because the Rancher node driver discovers cloned node addresses from the guest agent, and falls back to the NIC DHCP lease. Image nodes still reserve a static address. `external_network_id` is `vnet_default_gateway`. Omit it for a network with no uplink.

Firewall rules are `vergeio_network_rule`, not `vergeio_network_rules`. The cloud controller adds its own `k8s-` translate rules later, and an authoritative rule list would delete them. The rules are incoming accept from the cluster CIDR, incoming TCP 6443, incoming TCP 9345, and outgoing TCP 443. `api_source_cidr` sets the source on 6443 and 9345. `ssh_source_cidr` adds incoming TCP 22. Every rule except the last is staged with `apply` false. The last rule refreshes a running network.

`vergeio_vm` `name-pool` is stopped, has an 8 GB blank disk, and has no NIC. The CSI block driver keeps idle drives on that VM. `create_template` adds `name-template` for the Rancher node driver to clone. An image template is stopped, has the guest agent enabled, and reserves host 2. The driver clones that VM and replaces the clone's cloud-init. The published driver expects Ubuntu 24.04 with cloud-init. This module does not install that OS.

The API user is `type` `api`. `vergeio_api_key` sets `store_token` so the bearer token is kept and returned as `vergeos_api_key`. VergeOS returns that token only when the key is created. Grants are list, read, create, modify, and delete on `vms`, `machine_drives`, `machine_nics`, `cloudinit_files`, `machines`, `machine_status`, `vnets`, `vnet_rules`, `vnet_addresses`, `volumes`, `volume_nfs_shares`, and `vm_services`. Those are the collections the CSI driver, the cloud controller, and the node driver call. `vm_actions` and `vnet_actions` are not granted as their own tables. Hotplug and firewall apply post to those action endpoints.

`endpoint`, `cluster_ca_certificate`, `client_certificate`, and `client_key` are the kubernetes and helm provider arguments. `kubeconfig` is the same material. The golden image has to serve the API at `endpoint` with the certificate in `/etc/kubernetes/vergeio/server.crt` and the CA in `ca.crt`. This module does not install a Kubernetes distribution. `vergeos_host`, `vergeos_api_key`, and `vergeos_verify_ssl` are the `vergeos.*` values of the published charts. `vergeos_verify_ssl` is the string `true` or `false`. The published 0.2.0 charts default it to `false`.

```hcl
module "cluster" {
  source = "../../modules/kubernetes_cluster"

  name            = "edge"
  source_type     = "image"
  media_source_id = data.vergeio_mediasources.ubuntu.mediasources[0].id
  boot_disk_size  = 40
  cidr            = "192.168.100.0/24"
  vergeos_host    = "https://vergeos.example.com"
  powerstate      = true

  control_plane_count = 1
  worker_count        = 1
}
```

Runnable root modules are in `examples/modules/kubernetes_cluster/upstream` and `examples/modules/kubernetes_cluster/rancher`.

`terraform test` in this directory plans with a mocked provider. The lab apply test is a separate root in `tests/lab` and is not part of that run. It creates only `tf-acc-k8s` objects, leaves the network and the VMs stopped, and does not read or change an existing tenant:

```bash
terraform -chdir=tests/lab init -backend=false
terraform -chdir=tests/lab test -parallelism=1
```
