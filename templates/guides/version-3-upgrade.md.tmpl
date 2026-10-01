---
page_title: "Upgrade to v3.0"
description: |-
  Breaking changes between VergeIO provider 2.x and 3.0.
---

# Upgrade to v3.0

Provider 3.0 adds `vergeio_vm_drive` and `vergeio_vm_nic` and stops nesting drives and NICs on `vergeio_vm`. One data source was renamed. Configurations that set the arguments below as strings, that still use `vergeio_drive` or `vergeio_nic` blocks, or that depend on the old power-off and membership update behavior, need edits before the first 3.0 plan. Terraform upgrades existing state on the first refresh or apply. Copy the state file first. A string that cannot be converted fails that refresh, and Terraform does not write the upgraded state.

Pin 2.x with `version = "~> 2.7"` until the configuration and state are ready, then pin `version = "~> 3.0"`.

```terraform
terraform {
  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}
```

`~> 0.1.0` does not match any published release. The registry line for 2.x is 1.0.0 through 2.7.8.

## Arguments that were never in the schema

Product pages that show `network_address`, `dhcp_end`, or `dns_server_list` on `vergeio_network` do not match the provider. Both 2.x and 3.0 use `network` for the CIDR and `dhcp_stop` for the end of the DHCP range. 3.0 adds `description`, `domain`, `rate_limit`, and `dnslist`. `dnslist` is the API field for DNS servers handed to clients (a comma-separated list of addresses). It is not `dns_server_list`. 2.x has none of those four arguments. `vergeio_user` creates a user account. It does not manage storage.

The 3.0 provider registers these objects:

| Kind | Names |
| --- | --- |
| Resources | `vergeio_group`, `vergeio_member`, `vergeio_network`, `vergeio_network_rule`, `vergeio_network_rule_alias`, `vergeio_network_rules`, `vergeio_permission`, `vergeio_snapshot_profile`, `vergeio_tag`, `vergeio_tag_category`, `vergeio_tag_member`, `vergeio_tenant`, `vergeio_tenant_node`, `vergeio_tenant_storage`, `vergeio_user`, `vergeio_vm`, `vergeio_vm_drive`, `vergeio_vm_nic` |
| Data sources | `vergeio_cloudinit_files`, `vergeio_clusters`, `vergeio_groups`, `vergeio_mediasources`, `vergeio_networks`, `vergeio_nodes`, `vergeio_resource_groups`, `vergeio_tags`, `vergeio_tenants`, `vergeio_users`, `vergeio_version`, `vergeio_vms` |

Devices stay nested on `vergeio_vm`. Drives and NICs do not.

## Renamed data source

`vergeio_cloudinitfiles` is now `vergeio_cloudinit_files`. Change the data source address in configuration. Terraform drops the old address from state and reads the new one. `filter_name` and `cloudinit_files` are unchanged.

```terraform
data "vergeio_cloudinit_files" "all" {
}
```

## Type changes

Write numbers and bools without quotes. State that already stores a numeric string, or a network `powerstate` of `true`, `false`, `running`, or `stopped`, is rewritten in place. A blank string becomes null. Any other string fails the upgrade.

| Attribute | 2.x | 3.0 |
| --- | --- | --- |
| `vergeio_network.powerstate` | string (`"true"`, `"false"`) | bool |
| `vergeio_vm.cluster` | string | number (cluster key) |
| `vergeio_vm.preferred_node` | string | number (node key) |
| `vergeio_vm.snapshot_profile` | string | number (profile key) |
| `vergeio_vm_drive.preferred_tier` | string (`"1"` through `"5"`) on the old inline drive | number `1` through `5` |
| `vergeio_vms.vms[].drives[].preferred_tier` | string | number |
| `vergeio_vms.vms[].nics[].vnet` | string | number (vNET key) |

`vergeio_vm.powerstate` was already a bool in 2.x.

```terraform
resource "vergeio_network" "example" {
  name       = "Example Net"
  network    = "192.168.0.0/24"
  dhcp_stop  = "192.168.0.200"
  powerstate = false
}

resource "vergeio_vm" "example" {
  name             = "example"
  cluster          = 1
  preferred_node   = 2
  snapshot_profile = 4

  boot_disk {
    name = "os"
    size = 40
  }
}

resource "vergeio_vm_drive" "data" {
  vm_id          = vergeio_vm.example.id
  name           = "data"
  disksize       = 100
  preferred_tier = 3
}
```

Outputs and modules that pass the VM data source `preferred_tier` or NIC `vnet` into a string argument need a number argument instead.

`vergeio_vm.snapshot_profile` is the profile key. `vergeio_snapshot_profile` creates that profile. Set `snapshot_profile = tonumber(vergeio_snapshot_profile.example.id)`.

## Drives and NICs

`vergeio_drive` and `vergeio_nic` blocks are gone. A VM keeps machine settings, devices, and one optional `boot_disk`. Every other drive is a `vergeio_vm_drive`. Every NIC is a `vergeio_vm_nic`. Each one sets `vm_id` to the VM id and is matched by `name`.

The state upgrade rewrites numeric ids, then removes the inline drive and NIC lists from the VM. It does not delete those objects in VergeOS, and it does not put a drive into `boot_disk`. The next apply adopts a `boot_disk`, `vergeio_vm_drive`, or `vergeio_vm_nic` whose name already exists on that VM instead of creating a second device. A drive or NIC you drop from configuration and do not declare again stays in VergeOS until you delete it there.

Set `boot_disk.name` to the existing disk name. The default name is `boot`. A one-disk VM whose disk is not named `boot` gets a second disk if the name is left out.

A Terraform `moved` block addresses a whole resource. It cannot pull one nested block out of `vergeio_vm`. Moving `vergeio_vm` to `vergeio_vm_drive` or `vergeio_vm_nic` fails with that explanation and leaves the VM in state. Record an existing device before apply with an import block. The import id is `<vm_id>/<name>` or the drive key or NIC id.

```terraform
import {
  to = vergeio_vm_drive.data
  id = "15/data"
}

import {
  to = vergeio_vm_nic.lan
  id = "15/lan"
}

resource "vergeio_vm" "example" {
  name = "example"

  boot_disk {
    name = "os"
    size = 40
  }
}

resource "vergeio_vm_drive" "data" {
  vm_id    = vergeio_vm.example.id
  name     = "data"
  disksize = 100
}

resource "vergeio_vm_nic" "lan" {
  vm_id = vergeio_vm.example.id
  name  = "lan"
  vnet  = 6
}
```

`boot_disk` is the only drive `vergeio_vm` deletes or resizes. Do not give a `vergeio_vm_drive` the same name. Changing `boot_disk.media` or `boot_disk.source` replaces the VM. Changing `media` or `media_source` on `vergeio_vm_drive` replaces that drive. Destroying the VM still deletes its drives and NICs in VergeOS. Terraform destroys `vergeio_vm_drive` and `vergeio_vm_nic` first because they reference `vm_id`. A running guest that never releases the NIC is powered off with kill so the NIC can be deleted and destroy can continue to the VM.

Importing a VM imports machine settings only. Add `boot_disk` or the standalone resources and apply again so each device is adopted by name. `vergeio_vms` still returns drives and NICs on each VM. That data source is read-only and does not own them.

## Behavior changes

- Changing `group` or `member` on `vergeio_member` replaces the membership. Changing `tag_id` or `member` on `vergeio_tag_member` replaces the assignment. VergeOS does not update either object in place.
- `restart_on_change` on `vergeio_network` defaults to true. A DHCP range or address change restarts a running network so the staged settings become live. Set it to false to leave the network up. `need_restart` stays true until the network restarts. A stopped network is not restarted.
- Setting `vergeio_vm.powerstate` to false sends one ACPI poweroff and waits `timeouts.update`, which defaults to 2 minutes. It does not cut power. Set `force_power_off` to true to kill the VM if the guest does not stop. Guests without ACPI need `force_power_off` for that update.
- Omitting `powerstate` on an update leaves the current power state. A 2.x plan could power a running VM off when the argument was left unset.
- Destroying or replacing a running VM sends one ACPI poweroff and waits `timeouts.delete`, which also defaults to 2 minutes, then kills the VM if it is still running. `shutdown_on_destroy` accepts `graceful_then_kill` (the default), `graceful` (fail the destroy instead of killing), or `kill` (cut power immediately, which is the 2.x destroy behavior).
- Omitted `cpu_cores` and `ram` on create are sent as 1 core and 1024 MiB. That is the same platform default 2.7.8 left for VergeOS to apply. The 3.0 state stores those numbers.

## Provider configuration

`host`, `username`, and `password` are optional. An existing block that sets them still works. Every argument falls back to an environment variable when it is omitted. A value in the block, including an explicit empty string, wins over the environment.

| Argument | Environment variable |
| --- | --- |
| `host` | `VERGEOS_HOST` |
| `username` | `VERGEOS_USERNAME` |
| `password` | `VERGEOS_PASSWORD` |
| `api_key` | `VERGEOS_API_KEY` |
| `insecure` | `VERGEOS_INSECURE` |
| `timeout` | `VERGEOS_TIMEOUT` (seconds, default 60) |

`api_key` is sent as a bearer token. When `api_key` and a username and password are both set, the API key is used. `VERGEOS_VERIFY_SSL=false` means the same thing as `insecure = true`. The provider fails during configuration, before the first API request, when `host` is missing or when neither an API key nor both a username and password are available. That error names each missing value.
