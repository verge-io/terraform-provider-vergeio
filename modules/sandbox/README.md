# sandbox

Builds an isolated copy of a tenant, or of the VMs that carry one tag, and removes that copy when the module is destroyed.

The source is only read. This module does not create a snapshot, change a snapshot, or change the source tenant, VM, NIC, or tag.

This module does no data masking. The copy contains the source data.

Pass `source_type` `vm` and `tag` (`name` or `category/name`) to copy each tagged VM. Set `vms` instead of `tag` to pass that inventory in directly and skip the VM and snapshot reads. Pass `tenant_snapshots` with `tenant_id` to skip the tenant snapshot read. `vergeio_vm_snapshots` lists that VM's snapshots. Omit `snapshot_name` to use the newest snapshot, the greatest `created` time and then the greatest key. `vergeio_vm_clone` clones the snapshot VM (`snap_machine`, `is_snapshot` true). It does not clone the live VM. `preserve_macs` is false. Every NIC on the copy is moved onto a new internal network before create returns. A copy with no NIC gets one virtio NIC named `lan` on that network.

That network is `type` `internal`, `layer2_type` `none`, `interface_vnet` 0, DHCP off, and powered off. `vnet_default_gateway` is unset, so the network has no uplink. `allow_external` defaults to false. Set it to true and set `external_network_id` to write that key to `vnet_default_gateway` and power the network on. That is the only opt-in to a route outside the sandbox.

Pass `source_type` `tenant` and `tenant_id` to copy a tenant. `vergeio_tenant_snapshots` lists the snapshots. The same `snapshot_name` rule picks one, and the apply fails when that snapshot is missing. `vergeio_tenant_copy` then calls `Tenants.Clone`. The pinned govergeos client takes `name`, `no_vnet`, `no_storage`, and `no_nodes`. It has no snapshot argument, so the copy is the current tenant, not the bytes stored in the snapshot. `snapshot_id` is checked and stored so the configuration names a snapshot that exists. `tenant_actions` `restore` with `params.snapshot` overwrites the source tenant. This module does not call it. A system snapshot can create a new tenant through `cloud_snapshot_tenant_actions` action `recover`. The pinned client does not expose that call.

`isolate` defaults to true and posts `isolateon` on the copy. The module does not post `isolateoff`. Set `allow_external` to true to skip `isolateon`. The copy then keeps whatever connectivity the clone copied, including an external route when the source has one.

Destroy deletes the copy and, for VMs, the sandbox network. It does not delete the source or the snapshot.

```hcl
module "pull_request" {
  source = "../../modules/sandbox"

  name        = "pr-128"
  source_type = "vm"
  tag         = "app/billing"
  cidr        = "192.168.91.0/24"
}
```

```hcl
module "pull_request" {
  source = "../../modules/sandbox"

  name          = "pr-128"
  source_type   = "tenant"
  tenant_id     = data.vergeio_tenants.billing.tenants[0].id
  snapshot_name = "nightly"
}
```

A runnable root module is in `examples/modules/sandbox`. `examples/modules/sandbox/workflows/sandbox.yml` applies that root when a pull request opens or updates, and destroys it when the pull request closes.

`terraform test` in this directory plans with a mocked provider. The lab apply test is a separate root in `tests/lab` and is not part of that run. It creates only `tf-acc-` objects:

```bash
terraform -chdir=tests/lab init -backend=false
terraform -chdir=tests/lab test
```
