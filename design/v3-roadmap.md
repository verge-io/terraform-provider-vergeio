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
* `govergeos#55` is closed. It tracked tenant network blocks and external IPs. This provider still does not expose those as resources. See #64 below.
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
* #59 is done inside this repository. `examples/resources/vergeio_network` uses `network`, `dhcp_stop`, and `dnslist`. It does not use `network_address`, `dhcp_end`, or `dns_server_list`. Example files do not pin `0.1.0`. The README lists the same 18 resources and 12 data sources `internal/provider/provider.go` registers. The upgrade guide covers every 3.0.0 breaking change. The two product pages named in #59 live in `verge-io/docs-vergeos` and were not edited. They are `learn/08-developer-devops/04-terraform-packer.md` and `automate/product-guide/tools-integrations/terraform-provider.md`. Those pages are still the ones a new user finds first, and they still need a change in that other repository. Until then, soft/dev examples and guides are the source of truth for v3.

### Core resources

* #68 is closed. Drives and NICs are standalone resources, matched by name on `vm_id`. `vergeio_vm` keeps machine settings, nested devices, and an optional `boot_disk`. A `moved` block cannot pull one old nested block out of the VM. The import id for a drive or NIC is the VM id, a slash, and the name.
* #62 is closed for the four fields the Ansible network module already had: `description`, `domain`, `rate_limit`, and `dnslist`. Each is sent only when the configuration sets it. The naming rule that landed keeps API names. The DHCP range still ends at `dhcp_stop`, not `dhcp_end`. DNS servers are the string `dnslist`, a comma separated list of addresses, not `dns_server_list`. References such as `interface_vnet` stay numeric keys. Network types were not split into separate resources.
* #63 is closed. `vergeio_network_rule` is one firewall rule, matched by name. `vergeio_network_rules` writes every rule that is not a system rule on one network, together, and refreshes once. `vergeio_network_rule_alias` is a named address or port group. Reference it as `alias:<id>` (the alias resource id). VergeOS rejects `alias:` plus the name. The provider passes the value through unchanged.
* #64 is closed for `vergeio_tenant`, `vergeio_tenant_node`, `vergeio_tenant_storage`, and data source `vergeio_tenants`. The tenants guide is two root modules: the parent creates the tenant, and a second configuration points at `ui_address` once that address exists. Network blocks, external IPs, and a layer 2 network handed to a tenant are not resources here, even though govergeos can assign a network block and an external IP. Doing that in the UI or with another client can leave `need_fw_apply` set on the parent external network. The tenant resources do not clear that flag.
* #65 is closed. `vergeio_group` creates a group. `vergeio_member` adds a user or another object to a group. `vergeio_permission` grants a user or a group list, read, create, modify, and delete on a table or one object. `vergeio_users` lists users.
* #66 is closed. `vergeio_tag_category` chooses which object types can use the category. `vergeio_tag` creates a tag in it. `vergeio_tag_member` assigns a tag. Deleting a category deletes its tags and their assignments. A `taggable_*` flag is sent only when the configuration sets it.
* #67 is closed. `vergeio_snapshot_profile` defines when snapshots are taken and how long they are kept. Each `period` sets the frequency, the time of day, a required retention in seconds, and whether the snapshot is quiesced. Periods do not include `skip_missed`. `vergeio_vm.snapshot_profile` is that profile's key.

Changing `group` or `member` on `vergeio_member`, or `tag_id` or `member` on `vergeio_tag_member`, replaces the object. VergeOS does not update either in place. Omitted `cpu_cores` and `ram` on VM create are sent as 1 core and 1024 MiB, and 3.0 state stores those numbers. `restart_on_change` on `vergeio_network` defaults to true.

## v3.1

Nothing in this section is in the provider. The issues are open.

* #69. Keep passwords and API keys out of state, with write only attributes and ephemeral resources.
* #70. Resources for API keys and auth sources.
* #71. `terraform query`, so an existing system can be discovered and imported in bulk.
* #72. Actions for operations that are not resources, including a snapshot before a change.
* #32. VMs from catalog recipes. Labelled `blocked`.
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
