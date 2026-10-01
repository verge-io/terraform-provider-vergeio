## 3.0.0 (Unreleased)

NOTES:

- Rebuilt on the Terraform Plugin Framework (protocol 6), Go 1.27.1, and govergeos. The [v3 upgrade guide](docs/guides/version-3-upgrade.md) is the migration reference for configurations and state coming from 2.x (registry 1.0.0 through 2.7.8).

BREAKING CHANGES:

- `vergeio_vm` no longer nests drives and NICs. It keeps machine settings, devices, and an optional `boot_disk`. Other drives are `vergeio_vm_drive` and NICs are `vergeio_vm_nic`, each matched by name on `vm_id`. State upgrade drops the old inline lists and leaves those objects in VergeOS. The next apply adopts a `boot_disk`, `vergeio_vm_drive`, or `vergeio_vm_nic` whose name already exists.
- Data source `vergeio_cloudinitfiles` is renamed to `vergeio_cloudinit_files`.
- `vergeio_network.powerstate` is a bool. `vergeio_vm.cluster`, `preferred_node`, and `snapshot_profile` are numbers. Drive `preferred_tier` is a number from 1 through 5. `vergeio_vms` reports drive `preferred_tier` and NIC `vnet` as numbers. A numeric string already in state, or a network `powerstate` of `true`, `false`, `running`, or `stopped`, is rewritten on the first refresh. Any other string fails that refresh.
- Changing `group` or `member` on `vergeio_member`, or `tag_id` or `member` on `vergeio_tag_member`, replaces the object.
- Setting `vergeio_vm.powerstate` to false sends one ACPI poweroff and waits `timeouts.update` (default 2 minutes). It does not cut power. `force_power_off` kills the VM if the guest does not stop. Omitting `powerstate` on an update leaves the current power state.
- Destroying or replacing a running VM sends one ACPI poweroff and waits `timeouts.delete` (default 2 minutes), then kills the VM if it is still running. `shutdown_on_destroy` accepts `graceful_then_kill` (the default), `graceful`, or `kill` (the 2.x destroy behavior).
- `restart_on_change` on `vergeio_network` defaults to true. A DHCP range or address change restarts a running network. A stopped network is not restarted.
- Omitted `cpu_cores` and `ram` on VM create are sent as 1 core and 1024 MiB, the same platform default 2.7.8 left for VergeOS to apply. The 3.0 state stores those numbers.
- Provider arguments are optional. Each falls back to a `VERGEOS_*` environment variable when it is omitted. A value in the provider block wins, including an explicit empty string. `api_key` is sent as a bearer token and is used when both an API key and a username and password are set. Configuration fails before the first API request when `host` is missing or when neither form of authentication is complete.

FEATURES:

- **New Resource:** `vergeio_vm_drive` and `vergeio_vm_nic`. A drive or NIC added while the VM is already running is hotplugged.
- **New Resource:** `vergeio_network_rule` (one firewall rule, matched by name), `vergeio_network_rules` (every non-system rule on one network, written together and refreshed once), and `vergeio_network_rule_alias` (a named address or port group referenced as `alias:<name>`).
- **New Resource:** `vergeio_tenant`, `vergeio_tenant_node`, and `vergeio_tenant_storage`. **New Data Source:** `vergeio_tenants`. Network blocks and external IPs are not resources.
- **New Resource:** `vergeio_group` and `vergeio_permission`. **New Data Source:** `vergeio_users`. A permission grants a user or a group list, read, create, modify, and delete on a table or one object.
- **New Resource:** `vergeio_tag_category` and `vergeio_tag`. Deleting a category deletes its tags and their assignments. A `taggable_*` flag is sent only when the configuration sets it.
- **New Resource:** `vergeio_snapshot_profile`. Each `period` sets the frequency, the time of day, a required retention in seconds, and whether the snapshot is quiesced. `vergeio_vm.snapshot_profile` is that profile's key.
- `vergeio_network` accepts `description`, `domain`, `rate_limit`, and `dnslist`. Each is sent only when the configuration sets it. `dnslist` is the comma-separated list of DNS servers handed to clients.
- Drive `interface` accepts the machine_drives values current VergeOS reports, including `usb` and `nvme`. Apply still rejects a value the connected system does not offer.
- `vergeio_network.powerstate` follows the network running flag: power on at create, power on or kill on update, and kill before delete.

BUG FIXES:

- `vergeio_vm` refresh reads cloud-init file contents from VergeOS. A file edited or deleted outside Terraform shows in the plan, and a later update restores it.
- `vergeio_cloudinit_files`, `vergeio_resource_groups`, and `vergeio_tags` return an empty list when nothing matches.
- `vergeio_cloudinit_files` loads file contents from the download API. A `filter_name` that matches nothing returns an empty list.
- Editing `cloudinit_files` on an existing VM creates, updates, and deletes the file rows to match the plan.
- Changing `ipaddress_type` on an existing `vergeio_network` is sent on update. A later read stores the type VergeOS reports.
- Adding `boot_disk` to a VM already in state plans `boot_disk.key` as unknown so apply can store the drive key.
- `vergeio_network_rules` accepts a non-empty `rule` list. Nested rules use the nested schema, which does not include `vnet` or `apply`.
- `vergeio_snapshot_profile` periods no longer include `skip_missed`. VergeOS snapshot profile periods have no such column.
- Apply corrects `vergeio_vm.powerstate` when the configured value differs from the machine running flag, including a change made outside Terraform.
- A govergeos client that cannot be created is returned as a diagnostic. The provider does not keep a nil client.
- A VM create that fails after the VM exists in VergeOS keeps that id in state. A later create that collides on the name adopts the VM when it matches the plan. User and console passwords are not written to debug logs.
