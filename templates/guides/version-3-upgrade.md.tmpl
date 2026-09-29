---
page_title: "Upgrade to v3.0"
description: |-
  Breaking changes between VergeIO provider 2.x and 3.0.
---

# Upgrade to v3.0

Provider 3.0 keeps the same five resources as 2.7.8. One data source was renamed. Configurations that set the arguments below as strings, or that depend on the old power-off and membership update behavior, need edits before the first 3.0 plan. Terraform upgrades existing state on the first refresh or apply. Copy the state file first. A string that cannot be converted fails that refresh, and Terraform does not write the upgraded state.

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

Product pages that show `network_address`, `dhcp_end`, or `dns_server_list` on `vergeio_network` do not match 2.x or 3.0. Both lines use `network` for the CIDR and `dhcp_stop` for the end of the DHCP range. There is no DNS server argument on the network resource. `vergeio_user` creates a user account. It does not manage storage.

The 3.0 provider registers these objects:

| Kind | Names |
| --- | --- |
| Resources | `vergeio_member`, `vergeio_network`, `vergeio_tag_member`, `vergeio_user`, `vergeio_vm` |
| Data sources | `vergeio_cloudinit_files`, `vergeio_clusters`, `vergeio_groups`, `vergeio_mediasources`, `vergeio_networks`, `vergeio_nodes`, `vergeio_resource_groups`, `vergeio_tags`, `vergeio_version`, `vergeio_vms` |

Drives, NICs, and devices stay nested on `vergeio_vm`.

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
| `vergeio_drive.preferred_tier` | string (`"1"` through `"5"`) | number `1` through `5` |
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

  vergeio_drive {
    name           = "os"
    preferred_tier = 3
  }
}
```

Outputs and modules that pass the VM data source `preferred_tier` or NIC `vnet` into a string argument need a number argument instead.

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
