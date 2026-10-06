# v3 roadmap

This is the design note issue #87 asked for. The lists below are the index: what v3.0 was supposed to finish, what is actually on soft/dev, and what is still later work. Each issue still has one type label (`bug`, `enhancement`, or `documentation`) and, where it applies, `blocker`, `major`, `blocked`, or `deferred`. That is the same scheme as the Ansible collection.

If a GitHub milestone and this note disagree, the milestone wins.

Status was checked against issue state while preparing the 3.0 notes, on the soft/dev tip that pins govergeos commit `6b48c911`.

## Where the code lives

`Version2` is still the default branch and the line registry releases come from. `soft/dev` is the v3 integration tip. Feature and fix pull requests target `soft/dev`. Squash merge them there. Merging `soft/dev` into `Version2` or `main` waits for Justin's explicit sign off. `main` is behind `Version2`.

#55 is closed. The decision that landed is `soft/dev`, not a branch named `dev`. #41 closed from Linear and did not make `main` the release line. Cutting `release/2.x` from the last 2.x tag is still not done. `CONTRIBUTING.md` is the short version of that.

Resources and data sources live in `internal/services/<domain>`. `internal/provider` registers them. `internal/client` is the only package that imports `net/http` or builds a request URL. It also builds the govergeos client. Docs under `docs/` come from `templates/` via `make generate`. Examples under `examples/` are the configurations the registry pages render. Unit tests parse those files and reject an argument the schema does not define.

## What Terraform should own

Terraform is the right place for objects that stay around and can drift. On this branch that means the VM, its boot disk, extra drives, NICs, networks, firewall rules and aliases, tenants, the nodes and storage given to a tenant, users, groups, members, permissions, tags, and snapshot profiles.

Ansible stays the right place for step by step work inside a guest, and for platform objects this provider still does not model. The collection already set a network description, DNS servers, a domain, and a rate limit. The provider sends those four now, under the API names, and only when the configuration sets them.

A written handoff between the two tools is #79. A published modules repository is #77. Both are still open.

## govergeos

The provider talks to VergeOS through govergeos. The comment on #87 pointed at the SDK index `verge-io/govergeos#67` and said provider v3.0 depended on govergeos v0.4.0, with v3.1 on govergeos v0.5.0. That SDK index is still open. This branch does not pin either of those versions, and this note does not invent a tag for them.

`go.mod` requires `github.com/verge-io/govergeos v0.3.2-0.20261003022521-6b48c911a35c`. That string is a Go pseudo version for commit `6b48c911` on govergeos main. The 3.0.0 changelog says the commit is not a release tag.

The same comment named the SDK issues that mattered here. Their state when this note was written:

* `govergeos#41` is closed. It tracked a v0.3.1 that had been merged and not tagged. This provider still pins a commit, not a release tag.
* `govergeos#43` is closed. It tracked `PowerOff` as a hard kill, the same class of bug as #51. Setting `vergeio_vm.powerstate` to false sends one ACPI power off and waits `timeouts.update` (default 2 minutes). It does not cut power. `force_power_off` kills the VM if the guest does not stop. Destroy waits `timeouts.delete` and then follows `shutdown_on_destroy`.
* `govergeos#42` is closed. It tracked a name filter that could resolve a different object when the name contained braces.
* `govergeos#55` is closed. It tracked tenant network blocks and external IPs. `vergeio_tenant_external_ip` assigns the external IP (#210). `vergeio_tenant_network_block` assigns the network block (#211).
* `govergeos#50` and `govergeos#51` are closed. They tracked `WithEnvConfig` and option order. This provider reads `VERGEOS_*` itself and passes host, TLS, timeout, and either an API key or a username and password as explicit govergeos options. It does not call `WithEnvConfig`.

## v3.0, the foundation release

v3.0 is the only v3 release allowed to break configurations. The breaking list users should read is `BREAKING CHANGES` under `## 3.0.0 (Unreleased)` in `CHANGELOG.md`. The same list is written for operators in `templates/guides/version-3-upgrade.md.tmpl` (rendered to `docs/guides/version-3-upgrade.md`). The 3.0.0 changelog already records the imported machine type alias fix (#237) and the empty import id rejection on twelve resources (#235). Those lines match the tests and were left as written.

### Blockers

These are closed.

* #55. Integration branch is `soft/dev`. The release line question is still open. See Open decisions.
* #54. The provider is on govergeos. That work started from pull request #45.
* #53. Unit tests, an acceptance harness, and CI on pull requests are in place. The nightly schedule in `.github/workflows/acceptance.yml` is still commented out until the lab secrets exist. Older member acceptance tests, and the tag member acceptance tests, are still skipped in code. Group, permission, and member coverage in the access acceptance test runs when the lab is configured.
* #50. Importing a VM stores machine settings. It does not try to put drives and NICs back inside `vergeio_vm`. The next apply adopts a `boot_disk`, `vergeio_vm_drive`, or `vergeio_vm_nic` whose name already exists. An imported VM whose configuration says `machine_type` `q35` or `pc` no longer plans an update on every refresh once the boot disk is adopted (#237).
* #16. A VM create that fails after the VM exists in VergeOS keeps that id in state. A later create that collides on the name adopts the VM when it matches the plan.
* #51. `powerstate` false is an ACPI power off, not an immediate kill. See the govergeos note above for the exact wait.
* #52. Omitting `enabled` on `vergeio_vm_nic` leaves the VergeOS default, which is enabled. Set false to create the NIC disabled.

### Foundation

* #60 is closed. The provider is on the Terraform Plugin Framework (protocol 6) and Go 1.27.1.
* #61 is closed. Domain packages, one client package, an acceptance package, and generated docs are the layout above.
* #47 and #58 are closed. Every provider argument is optional and falls back to a `VERGEOS_*` variable when it is omitted. A value in the provider block wins, including an explicit empty string. `api_key` is sent as a bearer token and is used when both an API key and a username and password are set. Configuration fails before the first API request when `host` is missing or when neither form of authentication is complete.
* #56 is closed. The bug was a plan that showed drive keys, NIC ids, MAC addresses, and layer 2 ids as already known, so a reviewer could not tell an update from a replacement.
* #57 is closed. `vergeio_network.powerstate` is a bool. `vergeio_vm.cluster`, `preferred_node`, and `snapshot_profile` are numbers. Drive `preferred_tier` is a number from 1 through 5. `vergeio_vms` reports drive `preferred_tier` and NIC `vnet` as numbers.
* #34 is closed. Drive `interface` accepts the `machine_drives` values the connected VergeOS reports, including `usb` and `nvme`. Apply rejects a value that system does not offer.
* #59 is done inside this repository. `examples/resources/vergeio_network` uses `network`, `dhcp_stop`, and `dnslist`. It does not use `network_address`, `dhcp_end`, or `dns_server_list`. Example files do not pin `0.1.0`. The README lists the same 21 resources and 12 data sources `internal/provider/provider.go` registers. The upgrade guide covers every 3.0.0 breaking change. The two product pages named in #59 live in `verge-io/docs-vergeos` and were not edited. They are `learn/08-developer-devops/04-terraform-packer.md` and `automate/product-guide/tools-integrations/terraform-provider.md`. Those pages are still the ones a new user finds first, and they still need a change in that other repository. Until then, soft/dev examples and guides are the source of truth for v3.

### Core resources

* #68 is closed. Drives and NICs are standalone resources, matched by name on `vm_id`. `vergeio_vm` keeps machine settings, nested devices, and an optional `boot_disk`. A `moved` block cannot pull one old nested block out of the VM. The import id for a drive or NIC is the VM id, a slash, and the name.
* #62 is closed for the four fields the Ansible network module already had: `description`, `domain`, `rate_limit`, and `dnslist`. Each is sent only when the configuration sets it. The naming rule that landed keeps API names. The DHCP range still ends at `dhcp_stop`, not `dhcp_end`. DNS servers are the string `dnslist`, a comma separated list of addresses, not `dns_server_list`. References such as `interface_vnet` stay numeric keys. Network types were not split into separate resources.
* #63 is closed. `vergeio_network_rule` is one firewall rule, matched by name. `vergeio_network_rules` writes every rule that is not a system rule on one network, together, and refreshes once. `vergeio_network_rule_alias` is a named address or port group. Reference it as `alias:<id>` (the alias resource id). VergeOS rejects `alias:` plus the name. The provider passes the value through unchanged.
* #64 is closed for `vergeio_tenant`, `vergeio_tenant_node`, `vergeio_tenant_storage`, and data source `vergeio_tenants`. The tenants guide is two root modules: the parent creates the tenant, and a second configuration points at `ui_address` once that address exists. #210 adds `vergeio_tenant_external_ip`, one parent external IP owned by the tenant. The first assigned IP becomes `ui_address` on the next plan. #214 lets `vergeio_tenant.ui_address_id` choose which assigned external IP is the UI address, on create and on update. Omit it and the first assigned IP stays the default. A parent UI move is drift, and the next apply restores the configured value. `ui_address` remains the IP string. #211 adds `vergeio_tenant_network_block`, one routed CIDR (`vnet_cidrs`) from a parent network assigned to the tenant. `parent_firewall_pending` reports `need_fw_apply` on the parent network, and `apply_parent_firewall` applies those rules. VergeOS refuses to delete a block while a tenant network still uses it, and the resource returns that error. #212 adds `vergeio_tenant_layer2_network`, one parent layer 2 network bridged into the tenant (`tenant_layer2_vnets`). It requires VergeOS 26.0 or later. Destroy disables the assignment, then deletes it. Networks created inside the tenant remain after that host-side delete and belong to the tenant-side configuration. Leaving those components in place can block a later recreation. An assignment made in the UI or with another client can leave `need_fw_apply` set. The tenant resource does not clear that flag.
* #65 is closed. `vergeio_group` creates a group. `vergeio_member` adds a user or another object to a group. `vergeio_permission` grants a user or a group list, read, create, modify, and delete on a table or one object. `vergeio_users` lists users.
* #66 is closed. `vergeio_tag_category` chooses which object types can use the category. `vergeio_tag` creates a tag in it. `vergeio_tag_member` assigns a tag. Deleting a category deletes its tags and their assignments. A `taggable_*` flag is sent only when the configuration sets it.
* #67 is closed. `vergeio_snapshot_profile` defines when snapshots are taken and how long they are kept. Each `period` sets the frequency, the time of day, a required retention in seconds, and whether the snapshot is quiesced. Periods do not include `skip_missed`. `vergeio_vm.snapshot_profile` is that profile's key.

Changing `group` or `member` on `vergeio_member`, or `tag_id` or `member` on `vergeio_tag_member`, replaces the object. VergeOS does not update either in place. Omitted `cpu_cores` and `ram` on VM create are sent as 1 core and 1024 MiB, and 3.0 state stores those numbers. `restart_on_change` on `vergeio_network` defaults to true.

## v3.1

* #69 is closed. `password` on `vergeio_user` and `vergeio_tenant`, `console_pass` on `vergeio_vm`, and cloud-init `contents` stay through v3.x and are deprecated. The write-only forms are `password_wo`, `console_pass_wo`, and `contents_wo`, each paired with a version attribute. Terraform stores the version, not the secret. Increment the version to send the secret again. Changing the write-only value alone does not plan an update. A console password change updates the VM in place. Ephemeral `vergeio_api_key` mints a short-lived user API key. Close deletes it, and `ttl_seconds` expires it if Close does not run. The name is exclusive: Open deletes an existing key of that name for that user. The tenants guide uses that key for the tenant stack. Provider `password` and `api_key` cannot be write-only. Leave them out of the provider block and use `VERGEOS_USERNAME` and `VERGEOS_PASSWORD`, or pass the ephemeral token to `api_key`. Write-only attributes need Terraform 1.11 or OpenTofu 1.11. Ephemeral resources need Terraform 1.10 or OpenTofu 1.11. OpenTofu 1.10 and earlier reject both.

* #71 is closed. `terraform query` lists `vergeio_vm`, `vergeio_network`, `vergeio_tenant`, `vergeio_user`, `vergeio_group`, `vergeio_tag`, and `vergeio_snapshot_profile`. `name_pattern`, `tag`, and, on VMs and networks, `tenant` narrow the list. Snapshot VMs and snapshot tenants are omitted. The identity `id` is the existing import id, so an OpenTofu `import` block uses the same string. The adopt guide is the path from a query to a plan that only imports, then an empty plan.

* #72 is closed. `vergeio_vm_snapshot`, `vergeio_network_apply`, `vergeio_vm_power`, and `vergeio_tenant_snapshot` are Terraform 1.14 actions. Invoke them with `terraform apply -invoke` or a resource `lifecycle` `action_trigger`. They are not stored in state. OpenTofu does not implement actions, and no resource behavior depends on them. `vergeio_vm_snapshot` takes an instant VM snapshot. `vergeio_network_apply` refreshes firewall rules, DNS, or both on a running network. `vergeio_vm_power` shuts down, resets, or powers on a VM without changing `vergeio_vm.powerstate`. `vergeio_tenant_snapshot` snapshots a whole tenant.

* #32. `vergeio_vm_recipe_instance` deploys a VM from a catalog recipe. `answers` is a map of strings. Disk size answers are bytes. A bool answer VergeOS would store as false is rejected before the request is sent. There is no simulate argument. `vergeio_catalogs` and `vergeio_vm_recipes` look a recipe up by name. Managing recipes and catalogs as code is still later work.

The remaining issues in this section are open.

* #70. `vergeio_api_key` manages a long-lived user API key: name, description, expiry, and IP allow and deny lists. VergeOS returns the bearer token only on create. The resource does not store it. `ephemeral.vergeio_api_key` remains the short-lived token for one run, and Open still deletes an existing key of that name. Do not reuse a managed key's name there. `vergeio_auth_source` manages an external identity provider. `driver` is fixed at creation. `settings` is the non-secret JSON document. `client_secret_wo` is sent on create and when `client_secret_wo_version` changes, and it is not stored. An update merges settings into the stored document so a partial change does not wipe `client_secret` or other omitted keys. A key removed from `settings` stays on the auth source. Replace the auth source to drop it.
* #73. IPsec and WireGuard, including teardown order.
* #74. DNS zones, records, and views.
* #75. NAS services, volumes, and CIFS and NFS shares.
* #76. DR building blocks: sites, site sync, and cloud snapshots as resources.
* #77. A published modules repository (tenant, network segment, VM).
* #78. Certificates, webhooks, and system settings.
* #79. A Terraform and Ansible handoff guide.

## Deferred

These stay open and keep the `deferred` label. They are not part of the 3.0 release.

* #80. A sandbox per pull request, cloned from a tenant or from an application's VMs, destroyed on merge.
* #81. DR orchestration, with a rehearsed test failover and a gated real failover.
* #82. A compliance baseline module.
* #83. Fleet management across many systems and tenants.
* #84. A Kubernetes cluster module on the VergeOS CSI driver, cloud controller manager, and Rancher integration.
* #85. A nested lab module, for training labs and for disposable clusters that run acceptance tests.
* #86. VM import, export, and migration. Also labelled `blocked`, waiting on govergeos support.

## Open decisions

* Whether `main` becomes the release line, and whether to cut `release/2.x`. #55 settled the integration branch and left this. `Version2` stays the default until Justin says otherwise.
* Names versus keys. The drive and NIC model (#68) and the four network fields (#62) are decided as written above. Using a name where the API wants a numeric key did not land.
* Whether these milestones are mirrored in Linear. #41 was closed from Linear. This note does not assume a Linear board.
* Which lab system runs the nightly acceptance suite. The workflow stays manual until those secrets exist.
