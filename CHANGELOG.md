## 3.0.0 (Unreleased)

NOTES:

- Rebuilt on the Terraform Plugin Framework (protocol 6), Go 1.27.1, and govergeos. The [v3 upgrade guide](docs/guides/version-3-upgrade.md) is the migration reference for configurations and state coming from 2.x (registry 1.0.0 through 2.7.8).
- Built against govergeos commit `6b48c911` on main. That commit is not a release tag. An update that returns HTTP 404 keeps the VergeOS message. A missing row on read or delete is still reported as not found.

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
- **New Resource:** `vergeio_network_rule` (one firewall rule, matched by name), `vergeio_network_rules` (every non-system rule on one network, written together and refreshed once), and `vergeio_network_rule_alias` (a named address or port group referenced as `alias:<id>`).
- **New Resource:** `vergeio_tenant`, `vergeio_tenant_node`, and `vergeio_tenant_storage`. **New Data Source:** `vergeio_tenants`.
- **New Resource:** `vergeio_tenant_external_ip`. It assigns one virtual IP on a parent network to a tenant. The first assigned IP becomes the tenant UI address. The next plan of `vergeio_tenant` stores that address in `ui_address`. `apply_parent_firewall` applies the parent network rules on create, update, and delete. `parent_firewall_pending` reports `need_fw_apply` (#210).
- **New Resource:** `vergeio_tenant_network_block`. It assigns one routed CIDR (`vnet_cidrs`) from a parent network to a tenant. Changing `tenant_id`, `network_id`, `cidr`, or `description` replaces the block. `apply_parent_firewall` applies the parent network rules on create, update, and delete, then waits for `need_fw_apply` to clear. `parent_firewall_pending` reports that flag. Import uses the `vnet_cidrs` key. VergeOS refuses a delete while a tenant network still uses the block, and Terraform returns that error (#211).
- **New Resource:** `vergeio_group` and `vergeio_permission`. **New Data Source:** `vergeio_users`. A permission grants a user or a group list, read, create, modify, and delete on a table or one object.
- **New Resource:** `vergeio_tag_category` and `vergeio_tag`. Deleting a category deletes its tags and their assignments. A `taggable_*` flag is sent only when the configuration sets it.
- **New Resource:** `vergeio_snapshot_profile`. Each `period` sets the frequency, the time of day, a required retention in seconds, and whether the snapshot is quiesced. `vergeio_vm.snapshot_profile` is that profile's key.
- `vergeio_network` accepts `description`, `domain`, `rate_limit`, and `dnslist`. Each is sent only when the configuration sets it. `dnslist` is the comma-separated list of DNS servers handed to clients.
- Drive `interface` accepts the machine_drives values current VergeOS reports, including `usb` and `nvme`. Apply still rejects a value the connected system does not offer.
- `vergeio_network.powerstate` follows the network running flag: power on at create, power on or kill on update, and kill before delete.

BUG FIXES:

- `vergeio_tenant_external_ip` waits for the parent network `need_fw_apply` flag to clear after a successful firewall apply. VergeOS accepts the refresh before the flag drops, so an immediate read left `parent_firewall_pending` true (#210).
- Import of a `vergeio_vm` whose configuration says `machine_type = "q35"` or `machine_type = "pc"` no longer plans an update on every refresh. Import stores the expanded type VergeOS returns, such as `pc-q35-10.0` or `pc-i440fx-10.0`. That short name and the expanded type are the same machine type, so the plan is empty once `boot_disk` is adopted. A real change, including a different machine type, still plans (#237).
- An empty import id is rejected on `vergeio_vm`, `vergeio_user`, `vergeio_group`, `vergeio_member`, `vergeio_permission`, `vergeio_network`, `vergeio_tag_category`, `vergeio_tag`, `vergeio_tag_member`, `vergeio_tenant`, `vergeio_tenant_external_ip`, `vergeio_tenant_node`, and `vergeio_tenant_storage`. The error names the id that resource expects. A non empty id is still passed through (#235).
- Destroying more than one `vergeio_tenant_node` on a tenant retries when VergeOS returns 405 because only the last node can be deleted. Each node is removed once it is last. Sibling nodes stay up until their own destroy (#222).
- Setting `boot_disk.media` or `boot_disk.source` while adopting a disk after import, or after a 2.x upgrade, does not replace the VM. Changing either value on a disk Terraform already owns still replaces the VM (#198).
- `vergeio_tenant_storage.provisioned` must be a positive multiple of 1073741824 bytes (1 GiB). A value VergeOS would round down is rejected while planning (#197).
- `vergeio_user` updates keep `id` at the value already in state, so a `vergeio_member` or `vergeio_permission` that references that id is not replaced only because the user changed (#193).
- Changing `tpm_settings.version` on an existing VM replaces the VM so create can send the new version. Adding a TPM device stays an update. `"2.0"` and `"1.2"` are stored as `"2"` and `"1"`. Later updates omit the version because VergeOS treats it as read only after the device is created (#187, #177).
- `vergeio_network_rule_alias` refresh no longer treats an outside rename as a recycled key. Ownership uses the new computed `alias_id` (VergeOS readonly SHA1 hex), so a name change plans as in-place drift while a reused key with a different `alias_id` is still treated as gone (#231). The #229 name check conflated those cases and could create a duplicate alias.
- `vergeio_tenant` refresh no longer adopts a different tenant when VergeOS reuses the stored key after an outside delete. A mismatched computed `uuid` is treated as gone so the next apply creates instead of renaming and later destroying the foreign tenant (#232).
- `vergeio_tenant_storage`, `vergeio_tenant_node`, and `vergeio_network_rule_alias` refresh no longer adopt a foreign row when VergeOS reuses the stored key after an outside delete. A mismatched `tenant_id` (storage/node) or `alias_id` is treated as gone so the next apply creates instead of RequiresReplace-deleting the other object (#227).
- `vergeio_version` docs example output now shows a VergeOS 26+ version string and the real schema attributes (`name`, `version`, `hash`). The old example used VergeOS 4.x and a non-existent `id` attribute (#221).
- `vergeio_tenant` destroy waits for the tenant network (`Running=false`) after the tenant is offline, killing the vnet when needed, so `Tenants.Delete` no longer returns 405 while the network is still stopping (#205).
- `vergeio_tenant` create with `powerstate = true` before any `vergeio_tenant_node` exists defers power-on instead of waiting two minutes and leaving an orphan running tenant network (#207). A later apply powers the tenant on once a node exists.
- `vergeio_tenant_node` destroy stops only that node when it is running, instead of powering the whole tenant off, so removing one node from a multi-node online tenant leaves siblings running (#206). A running node is powered off first; if it does not stop within two minutes, it is killed (#220).

- Docs and examples for `vergeio_network_rule`, `vergeio_network_rules`, and `vergeio_network_rule_alias` now document alias references as `alias:<id>` (the alias resource id / `vnet_rule_aliases` key). VergeOS rejects `alias:<name>`; the provider passes the value through unchanged.
- `vergeio_vm` refresh detects when every configured `cloudinit_files` entry was deleted outside Terraform while those files were still expected to be live (for example after create with `powerstate = false`). Power-on detach still keeps an empty plan so the provider does not recreate files it removed after the first boot.
- `vergeio_vm` refresh seeds the cloud-init expect-live private marker when VergeOS still has attached `cloudinit_files` and the marker is absent (state upgraded from 2.x, or import). Those VMs then detect a later full external wipe like a 3.0 create; power-on detach still stays an empty plan.
- `vergeio_network.interface_vnet` set to `0` is treated as unset toward VergeOS (create and update omit it). Refresh stores `0` when VergeOS has no parent, so a configured `0` keeps a valid empty plan without rewriting the planned value.
- `vergeio_network` refresh stores `mtu`, `layer2_id`, `layer2_type`, `interface_vnet`, and `enable_bonding` from VergeOS. A configured MTU applies. A missing parent is stored as `interface_vnet = 0`.
- Changing the `rule` list on `vergeio_network_rules`, or adding a `vergeio_device` to an existing VM, no longer fails apply with "Provider produced inconsistent result". A nested id is taken from the object with the same name. `orderid` is kept only when that rule is still at the same index. A new object, or a rule that moved, is planned unknown.
- Deleting `vergeio_vm_nic` powers off a running VM whose guest leaves the NIC up after hot unplug, then deletes the NIC. Destroy no longer fails at the NIC and leaves the VM and network running. A guest that releases the NIC is left running.
- `vergeio_vm` refresh reads cloud-init file contents from VergeOS. A file edited outside Terraform, or deleted while another cloud-init file remains, shows in the plan, and a later update restores it.
- `vergeio_vm` cloud-init refresh keeps a configured name such as `user-data` when VergeOS stores `/user-data`, leaves `cloudinit_files` unset when the configuration sets none, and keeps files removed after the VM powers on. Those three no longer plan a change on every refresh.
- `vergeio_cloudinit_files`, `vergeio_resource_groups`, and `vergeio_tags` return an empty list when nothing matches.
- `vergeio_cloudinit_files` loads file contents from the download API. A `filter_name` that matches nothing returns an empty list.
- Editing `cloudinit_files` on an existing VM creates, updates, and deletes the file rows to match the plan.
- Changing `ipaddress_type` on an existing `vergeio_network` is sent on update. A later read stores the type VergeOS reports.
- Adding `boot_disk` to a VM already in state plans `boot_disk.key` as unknown so apply can store the drive key.
- `vergeio_network_rules` accepts a non-empty `rule` list. Nested rules use the nested schema, which does not include `vnet` or `apply`.
- `vergeio_snapshot_profile` periods no longer include `skip_missed`. VergeOS snapshot profile periods have no such column.
- Apply corrects `vergeio_vm.powerstate` when the configured value differs from the machine running flag, including a change made outside Terraform.
- A govergeos client that cannot be created is returned as a diagnostic. The provider does not keep a nil client.
- A VM create that fails after the VM exists in VergeOS keeps that id in state, including a boot disk or device failure, a power-on timeout, cloud-init detach, or a later read. A boot disk or device that already has a key is stored with the VM, so the next plan updates or replaces this VM instead of creating another one with the same name (#251). A later create that collides on the name adopts the VM when it matches the plan. User and console passwords are not written to debug logs.
