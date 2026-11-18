# vergeos_lab

Creates a nested VergeOS cluster on the host system: two core fabric networks and the node VMs for one shape. Each node is a `vergeio_vm` with nested virtualization, virtio-scsi disks, and a cloud-init install seed. Controller 1's seed creates the system. Controller 2 joins it.

`shape` is `hci`, `scale_out`, `hybrid`, or `uci`.

| shape | Nodes |
| --- | --- |
| hci | Two controllers |
| scale_out | Two controllers and `scale_out_count` scale-out nodes |
| hybrid | Two controllers and `compute_count` compute nodes |
| uci | Two controllers, `storage_count` storage nodes, and `compute_count` compute nodes |

Controller and scale-out nodes default to 8 cores, 16 GB of RAM, two 100 GB tier 0 disks, and two 1000 GB tier 1 disks. Set those smaller for a throwaway apply.

Counts must be known at plan time. Node keys are `controller-1`, `controller-2`, `scale-1`, `storage-1`, and `compute-1`. The VM name is `name` plus that key. Two labs need different values of `name`.

`vergeos_version` is recorded on each VM and as the first line of the seed. `media_source_id` is the install ISO for that version, from `vergeio_mediasources`. The provider cannot download an ISO. Omit `media_source_id` and the nodes have no CD-ROM. The seed still asks for a trial license (`YC_LICENSE_TYPE=trial`).

`powerstate` defaults to false, so the seed stays attached. Setting it true makes the provider detach cloud-init about a second after power-on, before the installer can read the seed. Leave it false and power the nodes on from the UI after apply, controller 1 first, then the node that joins it. `wait_for_guest_ip_timeout` is only accepted when `powerstate` is true. It waits for a guest-agent address and then continues. It does not mean the install finished.

`external_network_id` is a network the caller manages. This module does not look one up and does not change it. The external NIC does not request a host address. `external_ip` is the bare address the controller seed writes for the nested UI. `ui_address` repeats that address. Nothing checks that an API is listening there.

The core fabric networks are internal networks this module creates, named `<name>-core-1` and `<name>-core-2`. `ipaddress_type` is `none`. `ipaddress` is host 254 of the CIDR, a bare IP, so it is not a node address. Node N uses host N on each fabric. The defaults are `172.16.1.0/24` and `172.16.2.0/24`. Pass another `/24` when those networks already exist. The guest core overlay is `100.96.0.0/24`. Node N uses `100.96.0.(N+1)`.

`admin_password` needs an uppercase letter, a lowercase letter, a digit, and 8 characters. It is written with cloud-init `contents`, so Terraform stores the seed. `contents_wo` cannot be planned inside `cloudinit_files`.

Controller 1's seed writes a script the nested system runs after install. That script creates the storage and compute clusters when the shape needs them. The calls use the nested system's own network ids. They do not call the host, and they do not read or change a host tenant, user, vnet, or setting.

`cluster_id` places the VMs on a host cluster. Omit it to leave the system default. This module does not read `vergeio_clusters`.

Scale-out nodes ask the external network for DHCP. This module does not enable DHCP there.

`shutdown_on_destroy` is `kill`, so a node that was powered on is removed without waiting for ACPI. Destroy deletes the VMs, drives, NICs, and the two fabric networks. It does not delete the caller's external network or the ISO.

```hcl
module "lab" {
  source = "../../modules/vergeos_lab"

  name                = "training-1"
  shape               = "hci"
  vergeos_version     = "26.0.2"
  media_source_id     = data.vergeio_mediasources.iso.mediasources[0].id
  admin_password      = var.admin_password
  admin_email         = "admin@example.com"
  external_network_id = data.vergeio_networks.external.networks[0].id
  external_ip         = "198.51.100.10"
  external_prefix     = 24
  external_gateway    = "198.51.100.1"
  external_dns        = "198.51.100.1"
}
```

A runnable root module is in `examples/modules/vergeos_lab`. That root reads an existing ISO and an existing external network. The lab apply test does not.

`terraform test` in this directory plans with a mocked provider. The lab apply test is a separate root in `tests/lab` and is not part of that run. It creates only objects named with a `tf-acc-` prefix, writes no system settings, and does not read an existing vnet, media source, external IP, tenant, user, or setting. Run it with one operation at a time. The lab API drops connections under concurrent writes.

```bash
terraform -chdir=tests/lab init -backend=false
terraform -chdir=tests/lab test -parallelism=1
```
