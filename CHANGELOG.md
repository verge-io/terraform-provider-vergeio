## Unreleased

### Features

- **New Resource:** `vergeio_vm_import` and `vergeio_vm_export`. **New Data Source:** `vergeio_vm_import`. An OVA or OVF is imported as a VM and the apply waits until VergeOS reports complete or warning. A failed import returns as soon as the row is `error` or `aborted`. A disk image (`qcow2`, `vmdk`, `vhd`, `vhdx`, `raw`, `img`, `vdi`, `iso`) is downloaded into the media catalog with `target = "media"`, because VergeOS rejects that file on `vm_imports`. `file_id` is the catalog id a drive imports with `media = "import"`. Destroy removes the import row and the VM it created, unless `delete_vm` is false or a VM of that name already existed. A catalog file this resource downloaded is removed when `delete_file` is true. The data source reads one existing import by key or by name and waits, so a configuration can depend on an import started in the UI or by Ansible. A name that matches two rows is an error. `vergeio_vm_export` stores `quiesced`, `create_current`, and `max_exports` for one NAS volume. It does not start an export. Destroy removes that row and leaves files already written. The migration procedure, an export run, and the `image_pipeline` role stay in the Ansible collection. See [VM import and migration](docs/guides/vm-import.md) (#86).

### Bug fixes

- `terraform destroy -refresh=false` no longer deletes a different object that now holds a reused key. Before DELETE, `vergeio_site`, `vergeio_tag_category`, `vergeio_tag`, `vergeio_api_key`, and `vergeio_certificate` read the row and compare the stored `created` time. `vergeio_api_key` also compares the user. `vergeio_webhook_url` and `vergeio_snapshot_profile` compare the unique name, because VergeOS 26.2 and the pinned govergeos client have no `created` on those tables. A mismatch is already gone: destroy succeeds and leaves the other row, including its child rows. A missing row still follows the existing delete. A name or description change on the same row still deletes that row (#322).
- A second `vergeio_certificate` create or delete within a few seconds of another one no longer resets other creates in the same apply. VergeOS reloads the web server at once for one change, and about three seconds after the previous reload when another certificate change lands in that gap. The restart closes open connections. Other writes stay out until that later restart has finished and `/version.json` answers again. The read that follows create stays inside the same hold, so the next certificate change cannot reset it. POST is still not retried (#321).
- Changing `domain_list`, `key_type`, or `rsa_key_size` on a self-signed `vergeio_certificate` replaces the certificate. VergeOS stores a new `domainlist` without issuing the certificate again, and `renew` does not regenerate it. A later certificate create or delete rewrites the stored list from the issued certificate, so an in-place update comes back as drift. A `letsencrypt` or `manual` certificate still updates `domain_list` in place. A description change on a self-signed certificate still updates in place (#320).
- `vergeio_tag`, `vergeio_api_key`, and `vergeio_certificate` refresh no longer adopt a different row when VergeOS reuses the key. Ownership uses `created`, which VergeOS sets once and does not change. A different `created` time is treated as gone, so the next apply creates instead of renaming the other tag or API key, or replacing the other certificate. A name or description change on the same row still plans in place. `vergeio_api_key` still treats a reused key that belongs to a different user as gone, and now does the same when the key belongs to the same user but `created` differs. Import leaves `created` empty, and the first read stores it. `vergeio_tag` gains computed `created` (#326).
- Import of a `vergeio_webhook_url` with `retries` 0 stores 0, so the plan after import is empty. `timeout` 0 and `allow_insecure` false are stored the same way (#303).
- `data.vergeio_version` returns `name` and `hash` from VergeOS `/version.json`. `name` is the short API name. `hash` is the 40-character git commit of the running build. The provider copied the version string into `name` and left `hash` empty (#300).
- Deleting an IPsec configuration, an IPsec connection, a WireGuard interface or peer, a DNS view, zone, or record, or a firewall rule no longer treats a 404 as success when the row is still there. VergeOS can refuse that delete after the parent network is removed outside Terraform and still answer 404. The provider reads the row again and returns an error, so Terraform keeps the resource in state. A refresh of `vergeio_network_ipsec` or `vergeio_network_ipsec_connection` warns when the parent network is gone and the row remains (#292).
- Creating or deleting a `vergeio_certificate` no longer makes other creates in the same apply fail with "connection reset by peer" or EOF. VergeOS closes open API connections about half a second after those two calls. Other writes wait until that window has passed. The provider retries its own GET, PUT, and DELETE requests after a connection reset or EOF. POST is not retried (#295).
- `data.vergeio_vms` now returns drives and NICs on each VM. `allocated_bytes` and `filesize` stay null (#296).

## v3.0.0

### Breaking changes

- Rebuilt on the Terraform Plugin Framework (protocol 6), Go 1.27.1, and govergeos. The [v3 upgrade guide](docs/guides/version-3-upgrade.md) (`templates/guides/version-3-upgrade.md.tmpl`) is the migration reference for configurations and state coming from 2.x (registry 1.0.0 through 2.7.10).
- Built against govergeos v0.5.0 (`3f04250e`) plus `4986437a`, which reads a new API key secret from `response.private_key` and still accepts `response.token`. An update that returns HTTP 404 keeps the VergeOS message. A missing row on read or delete is still reported as not found. Cloud snapshot VM and tenant rows in that release expose the source as `original_key` and, for a VM, `machine_uuid`. This provider does not read those rows, so no Terraform attribute moves. A VM device `resource_group` is still the resource group UUID (#275).
- `vergeio_vm` no longer nests drives and NICs. It keeps machine settings, devices, and an optional `boot_disk`. Other drives are `vergeio_vm_drive` and NICs are `vergeio_vm_nic`, each matched by name on `vm_id`. State upgrade drops the old inline lists and leaves those objects in VergeOS. The next apply adopts a `boot_disk`, `vergeio_vm_drive`, or `vergeio_vm_nic` whose name already exists.
- Data source `vergeio_cloudinitfiles` is renamed to `vergeio_cloudinit_files`.
- `vergeio_network.powerstate` is a bool. `vergeio_vm.cluster`, `preferred_node`, and `snapshot_profile` are numbers. Drive `preferred_tier` is a number from 1 through 5. `vergeio_vms` reports drive `preferred_tier` and NIC `vnet` as numbers. A numeric string already in state, or a network `powerstate` of `true`, `false`, `running`, or `stopped`, is rewritten on the first refresh. Any other string fails that refresh.
- Changing `group` or `member` on `vergeio_member`, or `tag_id` or `member` on `vergeio_tag_member`, replaces the object.
- Setting `vergeio_vm.powerstate` to false sends one ACPI poweroff and waits `timeouts.update` (default 2 minutes). It does not cut power. `force_power_off` kills the VM if the guest does not stop. Omitting `powerstate` on an update leaves the current power state.
- Destroying or replacing a running VM sends one ACPI poweroff and waits `timeouts.delete` (default 2 minutes), then kills the VM if it is still running. `shutdown_on_destroy` accepts `graceful_then_kill` (the default), `graceful`, or `kill` (the 2.x destroy behavior).
- `restart_on_change` on `vergeio_network` defaults to true. A DHCP range or address change restarts a running network. A stopped network is not restarted.
- Omitted `cpu_cores` and `ram` on VM create are sent as 1 core and 1024 MiB, the same platform default 2.7.8 left for VergeOS to apply. The 3.0 state stores those numbers.
- Provider arguments are optional. Each falls back to a `VERGEOS_*` environment variable when it is omitted. A value in the provider block wins, including an explicit empty string. `api_key` is sent as a bearer token and is used when both an API key and a username and password are set. Configuration fails before the first API request when `host` is missing or when neither form of authentication is complete.

### Features

- **New Resource:** `vergeio_site`, `vergeio_site_sync_incoming`, and `vergeio_site_sync_outgoing`. A site is a remote VergeOS system used for site sync. Create does not ask VergeOS to build syncs. Incoming and outgoing syncs are declared separately, with a schedule of snapshot profile periods and a remote retention. `auth_password_wo` is sent when the site is created and when `auth_password_wo_version` changes, and it is not stored. `registration_code_wo` is required on an outgoing sync. VergeOS rejects a create that omits it, and Terraform does not store the code. Cloud snapshots use `vergeio_snapshot_profile`. There is no separate cloud snapshot profile. **New Data Source:** `vergeio_site_sync_incoming_status` and `vergeio_site_sync_outgoing_status`. `max_lag_seconds` fails the plan when the sync has not run, or when the last activity is older than that many seconds (#76).
- **New Resource:** `vergeio_network_ipsec` and `vergeio_network_ipsec_connection`. A network has one IPsec configuration. `mode = "advanced"` sends `strongswan_conf`, `ipsec_conf`, and `ipsec_secrets`. The read used here does not return those files, so Terraform keeps the configured text. A connection is one phase 1 and one phase 2. Destroy deletes every phase 2 under that phase 1, then the phase 1. Deleting phase 1 while a phase 2 row remains fails and then keeps failing. Destroy of the IPsec configuration is refused while a phase 1 row remains. Deleting the network does not remove these rows. `psk` is not returned, so Terraform keeps the configured key (#73).
- **New Resource:** `vergeio_network_wireguard` and `vergeio_network_wireguard_peer`. Creating the interface stages firewall rules named Accept WireGuard and leaves them unapplied. `apply` defaults to true and refreshes a running network. A stopped network is not refreshed. It loads staged rules when it starts. Destroy disables the interface and applies that interface's running network, then waits for the apply before the delete. A stopped network is deleted without that apply. Destroy is refused while a peer remains. Peer `configure_firewall` defaults to `site-to-site`. Changing the interface `configure_firewall` or `external_ip` replaces the interface (#73).
- **New Resource:** `vergeio_nas_service`, `vergeio_nas_volume`, `vergeio_nas_cifs_share`, and `vergeio_nas_nfs_share`. The service is deployed from the Services recipe, which creates the virtual machine and the NAS service together. `name` and `network_id` are required. Create uses 4 CPU cores and 4096 MB of RAM. `user` blocks are the accounts on that service. `password_wo` is sent when the user is created and when `password_wo_version` changes. VergeOS requires a character that is not a letter or digit. Terraform stores the version, not the password. A volume's `max_size` is bytes. Omit it on create and Terraform sends 1048576. Destroy disables the volume and waits until VergeOS allows the delete. If shares still block that delete, Terraform removes them and tries again. Snapshot volumes are left in place. Removing the service removes its virtual machine. If creating a CIFS or NFS share succeeds and a later read fails, Terraform deletes the share so it cannot block volume destroy. Volume snapshots and volume sync are not part of these resources (#75).
- **New Resource:** `vergeio_certificate`, `vergeio_webhook_url`, `vergeio_webhook`, and `vergeio_setting`. A certificate is a TLS certificate for the UI and API. `type` is `manual`, `letsencrypt`, or `self_signed`. Changing `type` replaces the certificate. Changing a configured `domain_name` replaces it too. Leaving `domain_name` out keeps the stored name. `private_key_wo` and `eab_hmac_key_wo` are sent on create and when their version changes, and they are not stored. `vergeio_webhook_url` is a destination for an event notification. `authorization_value_wo` is the credential and is not stored. `vergeio_webhook` is one message queued to that destination. VergeOS does not edit a delivery, so changing `message` or `webhook_url_id` sends a new message. A delivery expires after about 70 days. A missing row is dropped from state. `vergeio_setting` is one existing system setting, such as `max_connections`. Update writes only that key's value. Destroy sets the key back to `default_value` and leaves every other setting alone. It does not delete the settings row. `value_wo` is the write-only form (#78).
- `password` on `vergeio_user` and `vergeio_tenant`, `console_pass` on `vergeio_vm`, and cloud-init `contents` stay through v3.x and are deprecated. The write-only forms are `password_wo`, `console_pass_wo`, and `contents_wo`, each paired with a version attribute. Terraform stores the version, not the secret. Increment the version to send the secret again. Changing the write-only value alone does not plan an update. Ephemeral `vergeio_api_key` mints a short-lived user API key for one run. Close deletes it. `ttl_seconds` expires it if Close does not run. `name` is exclusive: Open deletes an existing key of that name for that user before creating a new one. Provider `password` and `api_key` cannot be write-only. Write-only attributes need Terraform 1.11 or OpenTofu 1.11. Ephemeral resources need Terraform 1.10 or OpenTofu 1.11 (#69).
- **New Resource:** `vergeio_vm_recipe_instance`. It deploys a VM from a catalog recipe. `answers` is a map of question name to string. A bool answer must be `true`, `false`, `yes`, `no`, `on`, `off`, `1`, or `0`. Anything else is rejected before the request is sent. A `disksize` answer is bytes. 50 GB is `53687091200`. A value above zero and under 1 MB is rejected. A network answer is a network name, a vnet key, or `__new_internal__`. A network name is resolved to a vnet key. `__new_internal__` is sent unchanged. There is no simulate argument. Destroy sends one ACPI poweroff, waits `timeouts.delete` (default 2 minutes), kills the guest if it is still running, deletes the VM, then deletes the recipe instance. A missing guest does not block that delete. A missing recipe instance still stops and deletes the VM stored in `vm_id`. **New Data Source:** `vergeio_catalogs` and `vergeio_vm_recipes`. `filter_name` is an exact name. A recipe lookup can be limited to one catalog. Each recipe includes its questions (#32).
- **New Action:** `vergeio_vm_snapshot`, `vergeio_network_apply`, `vergeio_vm_power`, and `vergeio_tenant_snapshot`. These are Terraform 1.14 actions, invoked with `terraform apply -invoke` or a resource `lifecycle` `action_trigger`. They are not resources and are not stored in state. OpenTofu does not implement actions, and no resource behavior depends on them. `vergeio_vm_snapshot` takes an instant VM snapshot. `vergeio_network_apply` refreshes firewall rules, DNS, or both on a running network when `vergeio_network_rule` uses `apply = false`. `vergeio_vm_power` shuts down, resets, or powers on a VM without changing `vergeio_vm.powerstate`; a later plan restores a declared powerstate. `power_on` posts poweron when the machine is not running, including when the VM `powerstate` column is already true. `vergeio_tenant_snapshot` snapshots a whole tenant (#72).
- **New Action:** `vergeio_tenant_clone`, `vergeio_tenant_reset`, `vergeio_tenant_node_migrate`, and `vergeio_tenant_node_power`. These are Terraform 1.14 actions, invoked the same way, and they are not stored in state. `vergeio_tenant_clone` copies a tenant. `name` is required. `no_vnet`, `no_storage`, and `no_nodes` skip the network, the storage, and the nodes. The copy is not imported into state. `vergeio_tenant_reset` restarts a tenant and does not change `vergeio_tenant`. `vergeio_tenant_node_migrate` moves a tenant node to `target_node`, a host node key. `vergeio_tenant_node_power` kills or resets a tenant node. VergeOS has a `poweroffmaintenance` tenant node action. The pinned govergeos client does not expose it, so that operation is not offered (#217).
- **New List Resource:** `vergeio_vm`, `vergeio_network`, `vergeio_tenant`, `vergeio_user`, `vergeio_group`, `vergeio_tag`, and `vergeio_snapshot_profile`. `terraform query` lists objects that already exist so they can be imported in bulk. `name_pattern` is a name glob or an exact name. `tag` is a tag name or `category/name`. `tenant` is a tenant key or name on VMs and networks. Snapshot VMs and snapshot tenants are omitted. Each of those managed resources has an identity `id`, the same string `terraform import` and an OpenTofu `import` block already use. See the [adopt guide](docs/guides/adopt-system.md) (#71).
- **New Resource:** `vergeio_api_key`. It manages a long-lived user API key: name, description, expiry, and IP allow and deny lists. VergeOS returns the bearer token only on create. The resource does not store it and has no token attribute. `ephemeral.vergeio_api_key` remains the short-lived token for one run. Do not reuse a name across the two. Changing `user_id` replaces the key (#70).
- **New Resource:** `vergeio_auth_source`. It manages an external identity provider. `driver` is fixed at creation. `settings` is the non-secret JSON document. `client_secret_wo` is sent on create and when `client_secret_wo_version` changes, and it is not stored. An update merges settings into the stored document so a partial change does not wipe `client_secret`. A key removed from `settings` stays on the auth source. Replace the auth source to drop it (#70).
- **New Resource:** `vergeio_vm_drive` and `vergeio_vm_nic`. A drive or NIC added while the VM is already running is hotplugged.
- **New Resource:** `vergeio_network_rule` (one firewall rule, matched by name), `vergeio_network_rules` (every non-system rule on one network, written together and refreshed once), and `vergeio_network_rule_alias` (a named address or port group referenced as `alias:<id>`).
- **New Resource:** `vergeio_network_dns_view`, `vergeio_network_dns_zone`, and `vergeio_network_dns_record`. `email` on a zone is an address such as `hostmaster@example.com`. VergeOS rejects a dotted name such as `hostmaster.example.com`. A record `value` can be a literal or `vergeio_vm_nic.ipaddress`. `vm_nic_id` reads that NIC address when `value` is omitted, for an A or AAAA record. A change sets `need_dns_apply`. `apply` defaults to true and refreshes DNS on a running network, the same way a firewall rule refreshes rules. Set `apply` to false to stage a batch, then leave it true on the last resource, which `depends_on` the others. A stopped network is not refreshed. It loads staged DNS when it starts. VergeOS deletes records with a zone and zones with a view. A refused delete removes the child rows first so they cannot block network delete (#74).
- **New Resource:** `vergeio_tenant`, `vergeio_tenant_node`, and `vergeio_tenant_storage`. **New Data Source:** `vergeio_tenants`.
- **New Resource:** `vergeio_tenant_external_ip`. It assigns one virtual IP on a parent network to a tenant. The first assigned IP becomes the tenant UI address. The next plan of `vergeio_tenant` stores that address in `ui_address`. `apply_parent_firewall` applies the parent network rules on create, update, and delete. `parent_firewall_pending` reports `need_fw_apply` (#210).
- **New Resource:** `vergeio_tenant_network_block`. It assigns one routed CIDR (`vnet_cidrs`) from a parent network to a tenant. Changing `tenant_id`, `network_id`, `cidr`, or `description` replaces the block. `apply_parent_firewall` applies the parent network rules on create, update, and delete, then waits for `need_fw_apply` to clear. `parent_firewall_pending` reports that flag. Import uses the `vnet_cidrs` key. VergeOS refuses a delete while a tenant network still uses the block, and Terraform returns that error (#211).
- **New Resource:** `vergeio_tenant_snapshot`. It keeps one snapshot of a tenant. Create takes the snapshot. `description` updates in place. `expires` and `never_expires` update the expiration in place. Set `expires` to a Unix timestamp, or set `never_expires` to true. Changing `tenant_id`, `name`, or `type` replaces the snapshot. VergeOS does not rename a snapshot, move it, or change its coverage after creation, and the update call has no name field, so a new name deletes the old row and creates another. Omit `name` and VergeOS assigns one. Omit `type` and VergeOS uses `full`. Import accepts the snapshot key or `tenant_id/name`. **New Data Source:** `vergeio_tenant_snapshots` lists that tenant's snapshots with name, coverage type, created, and expires. `type` is `full`, `partial_include`, or `partial_exclude`. The Provider and Local labels in the tenant UI are cloud snapshots, and this list does not include them. The `vergeio_tenant_snapshot` action still takes a snapshot and does not store it (#216).
- **New Resource:** `vergeio_tenant_layer2_network`. It bridges one parent layer 2 network into a tenant (`tenant_layer2_vnets`) on VergeOS 26.0 or later. Changing `tenant_id` or `network_id` replaces the assignment. `enabled` updates in place and defaults to true. Destroy disables the assignment, then deletes it. Networks created inside the tenant remain after that host-side delete and belong to the tenant-side configuration. Leaving those components in place can block a later recreation. Import uses the `tenant_layer2_vnets` key (#212).
- **New Resource:** `vergeio_tenant_node_device`. It attaches a host device to a tenant node so VMs inside the tenant can use it. `resource_group` is the resource group UUID from `vergeio_resource_groups`. `type` is the device type, including `node_pci_devices`, `node_host_gpu_devices`, and `node_nvidia_vgpu_devices`. `device_count` is sent as `count` when the device is created. Terraform reserves `count`, so the argument cannot use that name. It defaults to 1. The device row does not store it, and changing it replaces the device. A value above 1 creates that many rows. `device_ids` lists the copies with this name and type, and destroy removes those rows. A different device created on the node at the same time is not included. A failed lookup after create leaves those rows in place. The device is removed only when it is confirmed to belong to a different machine. `enabled` defaults to true. `optional` defaults to false and lets the node start when the device is missing. A device passed to a tenant is thick provisioned: the tenant owns it even when no VM inside the tenant is using it, and it cannot be assigned to another VM or tenant until this resource is destroyed. Changing `tenant_node_id` or `type` replaces the device. **New Data Source:** `vergeio_tenant_node_devices` lists the devices on one tenant node (#215).
- **New Resource:** `vergeio_group` and `vergeio_permission`. **New Data Source:** `vergeio_users`. A permission grants a user or a group list, read, create, modify, and delete on a table or one object.
- **New Resource:** `vergeio_tag_category` and `vergeio_tag`. Deleting a category deletes its tags and their assignments. A `taggable_*` flag is sent only when the configuration sets it.
- **New Resource:** `vergeio_snapshot_profile`. Each `period` sets the frequency, the time of day, a required retention in seconds, and whether the snapshot is quiesced. `vergeio_vm.snapshot_profile` is that profile's key.
- `vergeio_network` accepts `description`, `domain`, `rate_limit`, and `dnslist`. Each is sent only when the configuration sets it. `dnslist` is the comma-separated list of DNS servers handed to clients.
- Drive `interface` accepts the machine_drives values current VergeOS reports, including `usb` and `nvme`. Apply still rejects a value the connected system does not offer.
- `vergeio_network.powerstate` follows the network running flag: power on at create, power on or kill on update, and kill before delete.

### Bug fixes

- `terraform query` `tenant` on `vergeio_network` and `vergeio_vm` no longer sends `tenant eq`. Neither `vnets` nor `vms` has that column, and VergeOS answers an unknown column with status 200 and an empty list. Both lists send `owner eq 'tenants/<key>'`. A network the tenant owns, including the network created with the tenant, is `vnets.owner`. A VM is included when `vms.owner` is that reference. Guest VMs inside a tenant stay on that tenant's API (#297).
- Import of a missing `vergeio_nas_service`, `vergeio_nas_volume`, `vergeio_nas_cifs_share`, `vergeio_nas_nfs_share`, `vergeio_network_dns_view`, `vergeio_network_dns_zone`, `vergeio_network_dns_record`, `vergeio_network_ipsec`, `vergeio_network_ipsec_connection`, `vergeio_network_wireguard`, or `vergeio_network_wireguard_peer` reports that the remote object does not exist. A volume or share id that is not a 40 character SHA1 hex string, such as `abc`, is rejected in import with the id that resource expects. A plan after the object was deleted outside Terraform still plans a create (#299).
- A self-signed `vergeio_certificate` that sets `domain_name` is issued for that name. VergeOS ignores the read-only `domainname` column, so Terraform sends the name as the first `domain_list` entry and does not send `domainname`. Create and read wait until VergeOS fills `domain`, `domainname`, and `domain_list`, and return an error if those columns are still empty when the wait expires. A later read of a self-signed certificate that was issued for other names stores the issued name, so the next plan replaces it (#290).
- `terraform query` lists `vergeio_network` and `vergeio_vm` with `include_resource` and with `-generate-config-out`. Both failed with a Value Conversion Error on `bond_interfaces_args` or `guest_agent_ips` and returned no rows (#293).
- Ephemeral `vergeio_api_key` returns the bearer token on VergeOS 26.2. The create response puts that secret in `private_key`. govergeos v0.5.0 read `token` and returned an empty string, so Open deleted the new key and failed with "VergeOS did not return an API key token" on every run. The provider uses the govergeos commit that reads `private_key`. A create response with no secret still deletes the key (#294).
- A `vergeio_certificate` create that fails on the follow-up read keeps the certificate key in state. The next apply reads that row instead of posting another certificate (#268).
- `vergeio_tenant_clone` waits until `no_nodes` and `no_storage` have taken effect. VergeOS accepts the clone before the copy finishes, so the new tenant can still list the source node. The action reads until those rows are gone. A row that remains is removed, and the action does not report success while it is still there (#217).
- `vergeio_tenant.isolate` can be set in configuration. `true` calls isolate on and `false` calls isolate off. Omit it to leave the current isolation unchanged. Refresh reads the tenant row, so a change made in the parent UI is drift and the next apply restores the configured value (#213).
- `vergeio_tenant.ui_address_id` can be set in configuration. It chooses which assigned external IP is the tenant UI address, on create and on update. Omit it and the first assigned IP stays the UI address. `ui_address` remains the IP string. Refresh reads the tenant row, so a change made in the parent UI is drift and the next apply restores the configured value (#214).
- `vergeio_tenant_external_ip` waits for the parent network `need_fw_apply` flag to clear after a successful firewall apply. VergeOS accepts the refresh before the flag drops, so an immediate read left `parent_firewall_pending` true (#210).
- Import of a `vergeio_vm` whose configuration says `machine_type = "q35"` or `machine_type = "pc"` no longer plans an update on every refresh. Import stores the expanded type VergeOS returns, such as `pc-q35-10.0` or `pc-i440fx-10.0`. That short name and the expanded type are the same machine type, so the plan is empty once `boot_disk` is adopted. A real change, including a different machine type, still plans (#237).
- An empty import id is rejected on `vergeio_vm`, `vergeio_user`, `vergeio_group`, `vergeio_member`, `vergeio_permission`, `vergeio_network`, `vergeio_tag_category`, `vergeio_tag`, `vergeio_tag_member`, `vergeio_tenant`, `vergeio_tenant_external_ip`, `vergeio_tenant_node`, and `vergeio_tenant_storage`. The error names the id that resource expects. A non empty id is still passed through (#235). Resources added after that change reject an empty import id the same way.
- Destroying more than one `vergeio_tenant_node` on a tenant retries when VergeOS returns 405 because only the last node can be deleted. Each node is removed once it is last. Sibling nodes stay up until their own destroy (#222).
- Setting `boot_disk.media` or `boot_disk.source` while adopting a disk after import, or after a 2.x upgrade, does not replace the VM. Changing either value on a disk Terraform already owns still replaces the VM (#198).
- `vergeio_tenant_storage.provisioned` must be a positive multiple of 1073741824 bytes (1 GiB). A value VergeOS would round down is rejected while planning (#197).
- `vergeio_user` updates keep `id` at the value already in state, so a `vergeio_member` or `vergeio_permission` that references that id is not replaced only because the user changed (#193).
- Changing `tpm_settings.version` on an existing VM replaces the VM so create can send the new version. Adding a TPM device stays an update. `"2.0"` and `"1.2"` are stored as `"2"` and `"1"`. Later updates omit the version because VergeOS treats it as read only after the device is created (#187, #177).
- `vergeio_network_rule_alias` refresh no longer treats an outside rename as a recycled key. Ownership uses the new computed `alias_id` (VergeOS readonly SHA1 hex), so a name change plans as in-place drift while a reused key with a different `alias_id` is still treated as gone (#231). The #229 name check conflated those cases and could create a duplicate alias.
- `vergeio_site` and `vergeio_tag_category` refresh no longer adopt a different row when VergeOS reuses the key. Ownership uses `created`, which VergeOS sets once and does not change. A different `created` time is treated as gone, so the next apply creates instead of renaming the other row. A name change on the same row still plans in place. Import leaves `created` empty, and the first read stores it. `vergeio_tag_category` gains computed `created` (#291).
- `vergeio_webhook_url` and `vergeio_snapshot_profile` refresh no longer adopt a different row when VergeOS reuses the key. VergeOS 26.2 has no `created` time or other readonly id on those tables, and the pinned govergeos client does not return one. Ownership uses the unique name. A different name is treated as gone, including a rename made outside Terraform. Import leaves the name empty, and the first read stores the current name (#291).
- `vergeio_tenant` refresh no longer adopts a different tenant when VergeOS reuses the stored key after an outside delete. A mismatched computed `uuid` is treated as gone so the next apply creates instead of renaming and later destroying the foreign tenant (#232).
- `vergeio_tenant_storage`, `vergeio_tenant_node`, and `vergeio_network_rule_alias` refresh no longer adopt a foreign row when VergeOS reuses the stored key after an outside delete. A mismatched `tenant_id` (storage/node) or `alias_id` is treated as gone so the next apply creates instead of RequiresReplace-deleting the other object (#227).
- `vergeio_tenant` destroy waits for the tenant network (`Running=false`) after the tenant is offline, killing the vnet when needed, so `Tenants.Delete` no longer returns 405 while the network is still stopping (#205).
- `vergeio_tenant` create with `powerstate = true` before any `vergeio_tenant_node` exists defers power-on instead of waiting two minutes and leaving an orphan running tenant network (#207). A later apply powers the tenant on once a node exists.
- `vergeio_tenant_node` destroy stops only that node when it is running, instead of powering the whole tenant off, so removing one node from a multi-node online tenant leaves siblings running (#206). A running node is powered off first; if it does not stop within two minutes, it is killed (#220).

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

### Docs

- The v3 upgrade guide is [docs/guides/version-3-upgrade.md](docs/guides/version-3-upgrade.md), generated from `templates/guides/version-3-upgrade.md.tmpl`. It is the migration reference for configurations and state coming from 2.x (registry 1.0.0 through 2.7.10). Example provider blocks pin `~> 3.0` (#279).
- [Terraform and Ansible](docs/guides/terraform-ansible.md) says which objects Terraform owns and which procedures Ansible owns. Terraform owns tenants, networks and firewall policy, VM configuration, identity, and snapshot profiles. Ansible owns node maintenance, rolling updates, restore drills, export and import, reporting, key rotation, and guest OS configuration. A VM tagged `web` through `vergeio_tag_member` joins the inventory group `tag_web` from `vergeio.vergeos.vergeos_vms`. Provider 3.0 and the collection modules share `VERGEOS_HOST`, `VERGEOS_USERNAME`, `VERGEOS_PASSWORD`, `VERGEOS_API_KEY`, and `VERGEOS_INSECURE` (#79).
- [Adopt an existing VergeOS system](docs/guides/adopt-system.md) is the path from `terraform query` to import. The identity `id` is the same string `terraform import` and an OpenTofu `import` block already use (#71).
- Docs and examples for `vergeio_network_rule`, `vergeio_network_rules`, and `vergeio_network_rule_alias` document alias references as `alias:<id>` (the alias resource id / `vnet_rule_aliases` key). VergeOS rejects `alias:<name>`. The provider passes the value through unchanged (#194).
- The `vergeio_version` example shows a VergeOS 26+ version string and the schema attributes `name`, `version`, and `hash`. The old example used VergeOS 4.x and a non-existent `id` attribute (#221).

## v2.7.10 - 2026-10-08

### Fixed

- `LICENSE` is now the verbatim Apache 2.0 text from apache.org with only the copyright line filled in. The copy added in v2.7.7 carried three wording edits and a garbled appendix sentence, which made the OpenTofu registry scanner match the file as both Apache-2.0 and ECL-2.0 and flag v2.7.7 through v2.7.9 as `incompatible_license`. No code change. [#273](https://github.com/verge-io/terraform-provider-vergeio/pull/273)

## v2.7.9 - 2026-10-05

### Fixed

- `vergeio_vm`: an apply that sets `powerstate` now fails when the VM does not reach the requested state. The provider posts the `poweron` or `kill` action and polls the platform every 5 seconds, up to 25 seconds. Before this release it gave up silently after that window and returned success, so Terraform either recorded `powerstate = true` for a VM that never started or raised a framework "inconsistent result after apply" error that blamed the provider. The error now names the VM, the requested state, and the last observed state. On create the VM already exists on the platform when this error is raised but is not yet in Terraform state; the Power state section on the `vergeio_vm` page describes how to recover. [#248](https://github.com/verge-io/terraform-provider-vergeio/issues/248), [#249](https://github.com/verge-io/terraform-provider-vergeio/pull/249)

### Changed

- Removed comments that only restated the code and the scaffolding template notes left over from the provider template. No behavior change. [#246](https://github.com/verge-io/terraform-provider-vergeio/pull/246)

## v2.7.8 - 2026-08-12

### Added

- `vergeio_vm`: `on_power_loss` attribute sets the VM's power state after a host power loss. [#49](https://github.com/verge-io/terraform-provider-vergeio/pull/49)

## v2.7.7 - 2026-04-07

### Fixed

- `vergeio_vm`: disk imports with a fractional GB size were truncated; `disksize` is now a float. State written by earlier versions is upgraded in place. [#46](https://github.com/verge-io/terraform-provider-vergeio/pull/46)

### Changed

- Added the CLA and the full license text.

## v2.7.6 - 2026-03-12

### Changed

- `vergeio_vm`: cloud-init files are deleted after the VM powers on, so the VM does not depend on them on later boots. [#42](https://github.com/verge-io/terraform-provider-vergeio/pull/42)

## v2.7.5 - 2026-01-28

### Added

- `vergeio_tags` data source filters by category, which disambiguates tags that share a name across categories. [#40](https://github.com/verge-io/terraform-provider-vergeio/pull/40)

### Changed

- Documentation update. [#39](https://github.com/verge-io/terraform-provider-vergeio/pull/39)

## v2.7.4 - 2026-01-06

### Added

- Tag support: `vergeio_tag_member` resource and `vergeio_tags` data source. [#36](https://github.com/verge-io/terraform-provider-vergeio/pull/36)
- Machine types and other enumerations are read from the VergeOS system and cached, instead of being fixed in the provider. [#37](https://github.com/verge-io/terraform-provider-vergeio/pull/37)

## v2.7.3 - 2025-12-04

### Changed

- Disk import updates. [#29](https://github.com/verge-io/terraform-provider-vergeio/pull/29)
- Q35 and PC machine type plan modifier, so `q35` and `pc-q35-10.0` (and the PC equivalents) are treated as the same value. `readVM` updated to match. [#33](https://github.com/verge-io/terraform-provider-vergeio/pull/33)

## v2.7.2 - 2025-11-24

### Fixed

- `vergeio_vm`: machine type validation no longer relies on a static schema. [#31](https://github.com/verge-io/terraform-provider-vergeio/pull/31)

## v2.7.1 - 2025-07-30

### Fixed

- `vergeio_vm`: vGPU and NVIDIA device handling in the `vergeio_device` block, with documentation for VM devices.

## v2.7.0 - 2025-07-29

### Added

- `vergeio_vm`: `vergeio_device` block for USB, TPM, PCI and vGPU devices.

### Fixed

- Type fix in the device schema.

## v2.6.1 - 2025-06-30

### Fixed

- `vergeio_vm`: power state bug.

## v2.6.0 - 2025-06-12

### Changed

- `vergeio_vm`: `powerstate` is a boolean.

## v2.5.0 - 2025-04-29

### Added

- `vergeio_vm`: guest IP address reporting.

## v2.4.0 - 2025-04-22

### Added

- VLAN bonding on networks, with documentation.
- `vergeio_vm` resource enhancements, with documentation.

### Fixed

- Deleting a VM in the UI no longer breaks the next Terraform run.

## v2.3.0 - 2025-03-28

### Added

- `vergeio_vms` data source reports guest agent IP addresses, with a configurable wait.

## v2.2.2 - 2025-03-24

### Added

- `vergeio_vms` data source: media source on disks and MAC address on NICs.

### Fixed

- Index bug in the VM data source.
- Network documentation.

## v2.2.1 - 2025-03-19

### Changed

- Documentation update.

## v2.2.0 - 2025-03-17

### Added

- `vergeio_vms` data source includes drives and NICs.
- `vergeio_vm`: `advanced` attribute for key and value pairs.
- External VLAN support on networks.

### Fixed

- `vlan_default_gateway` handling on networks.

## v2.1.0 - 2025-02-04

### Added

- Automatic IP assignment on NICs.

## v2.0.1 - 2025-01-28

### Changed

- Registry index page.

## v2.0.0 - 2025-01-28

### Changed

- The provider was rebuilt on the Terraform Plugin Framework with protocol version 6. Configurations written for the 1.x provider need review; see the resource and data source pages under `docs/`.

Releases before 2.0.0 are listed on [GitHub Releases](https://github.com/verge-io/terraform-provider-vergeio/releases).
