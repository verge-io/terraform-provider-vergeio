# Terraform VergeIO Provider

Terraform provider plugin to integrate with VergeOS

## Support

VergeIO welcomes pull requests and responds to issues on a best-effort basis. VergeIO maintains public GitHub repositories for initiatives that help customers integrate the VergeIO platform with other third-party products. Support for these initiatives is handled directly via the GitHub repository. Issues and enhancement requests can be submitted in the Issues tab of each repository. Search for and review existing open issues before submitting a new issue.

## Example Usage

See the docs folder for examples

## Configuration Reference

Every argument is optional. A value in the provider block wins over the environment variable for the same setting. Authentication is an API key, or a username and password. When both are set, the API key is used.

| Argument | Environment variable | Description |
| --- | --- | --- |
| `host` | `VERGEOS_HOST` | Hostname or IP address for the system or tenant. |
| `username` | `VERGEOS_USERNAME` | Username. Required when `api_key` is unset. |
| `password` | `VERGEOS_PASSWORD` | Password for `username`. Required when `api_key` is unset. |
| `api_key` | `VERGEOS_API_KEY` | API key sent as a bearer token. |
| `insecure` | `VERGEOS_INSECURE` | Skip TLS certificate verification. Defaults to `false`. `VERGEOS_VERIFY_SSL=false` means the same thing. |
| `timeout` | `VERGEOS_TIMEOUT` | HTTP request timeout in seconds. Defaults to `60`. |

The provider fails during configuration when `host` is missing or when neither an API key nor both a username and password are available. The error names each missing value.

```
provider "vergeio" {
	host = "Hostname_or_ip"
	username = "my_user"
	password = "my_password"
	insecure = false
}
```

```
provider "vergeio" {
	host = "Hostname_or_ip"
	api_key = "my_api_key"
}
```

```
# VERGEOS_HOST, VERGEOS_API_KEY or VERGEOS_USERNAME and VERGEOS_PASSWORD
provider "vergeio" {}
```

A `password` or `api_key` set in the provider block is stored in state. Provider arguments cannot be write-only. Omit them and set `VERGEOS_USERNAME` and `VERGEOS_PASSWORD`, or pass an ephemeral token to `api_key`, to keep those credentials out of state.

Resource passwords are stored when the attribute is not write-only. `vergeio_user.password`, `vergeio_tenant.password`, `vergeio_vm.console_pass`, and cloud-init `contents` are deprecated through v3.x. Use `password_wo`, `console_pass_wo`, and `contents_wo` with the matching `*_version` attribute. Terraform stores the version, not the secret. Increment the version to send a new value. Changing the write-only value alone does not plan an update. A console password change updates the VM in place.

## Ephemeral resources

- vergeio_api_key

`vergeio_api_key` mints a short-lived user API key for one run. The token is not stored. Close deletes the key. `ttl_seconds` expires it if Close does not run. `name` is exclusive: Open deletes an existing key of that name for that user before creating a new one. Do not reuse a long-lived key name. The tenants guide uses this key for the configuration that manages the inside of a tenant.

Write-only attributes and this ephemeral resource behave the same on Terraform and OpenTofu once the CLI is new enough: the write-only value is null in state, and ephemeral results are not in state or in the plan. OpenTofu added both features in 1.11. Terraform added ephemeral resources in 1.10 and write-only attributes in 1.11.

## Actions

Actions require Terraform 1.14. OpenTofu does not implement them. Nothing a resource does depends on an action. Invoke one with `terraform apply -invoke=action.TYPE.LABEL`, or from a resource `lifecycle` `action_trigger`. An action is not stored in state.

- vergeio_vm_snapshot
- vergeio_network_apply
- vergeio_vm_power
- vergeio_tenant_snapshot

`vergeio_vm_snapshot` takes an instant VM snapshot. `name` and `retention_seconds` are optional. `quiesce` asks the guest agent to freeze filesystems. `vergeio_tenant_snapshot` snapshots a whole tenant. `vergeio_network_apply` refreshes firewall rules, DNS, or both on a running network. Use it when `vergeio_network_rule` sets `apply` to false. A stopped network is left unchanged and the action returns an error. `vergeio_vm_power` shuts down, resets, or powers on a VM without changing `vergeio_vm.powerstate`. `power_on` posts poweron when the machine is not running, including when the VM `powerstate` column is already true. A later plan of that VM restores a declared powerstate. `timeout_seconds` and `force` apply only to `shutdown`.

## Query

`terraform query` lists VMs, networks, tenants, users, groups, tags, and snapshot profiles that already exist. Filter with `name_pattern`, `tag`, and, for VMs and networks, `tenant`. The identity `id` is the import id. OpenTofu uses that same id in an `import` block. See [Adopt an existing VergeOS system](docs/guides/adopt-system.md).

## Resources

- vergeio_api_key
- vergeio_auth_source
- vergeio_group
- vergeio_member
- vergeio_nas_cifs_share
- vergeio_nas_nfs_share
- vergeio_nas_service
- vergeio_nas_volume
- vergeio_network
- vergeio_network_dns_record
- vergeio_network_dns_view
- vergeio_network_dns_zone
- vergeio_network_rule
- vergeio_network_rule_alias
- vergeio_network_rules
- vergeio_permission
- vergeio_snapshot_profile
- vergeio_tag
- vergeio_tag_category
- vergeio_tag_member
- vergeio_tenant
- vergeio_tenant_external_ip
- vergeio_tenant_layer2_network
- vergeio_tenant_network_block
- vergeio_tenant_node
- vergeio_tenant_storage
- vergeio_user
- vergeio_vm
- vergeio_vm_drive
- vergeio_vm_nic
- vergeio_vm_recipe_instance

Devices stay nested on `vergeio_vm`.

`vergeio_network_dns_view`, `vergeio_network_dns_zone`, and `vergeio_network_dns_record` manage DNS on a network. A record can take `value` from `vergeio_vm_nic.ipaddress`, or set `vm_nic_id` so Terraform reads that NIC address. A change sets `need_dns_apply`. `apply` defaults to true and refreshes DNS on a running network, the same way `vergeio_network_rule` refreshes firewall rules. Set `apply` to false to stage several DNS resources, then leave it true on the last one. A stopped network loads staged DNS when it starts.

`vergeio_nas_service` deploys the Services recipe, which creates the virtual machine and the NAS service together. `name` and `network_id` are required. `user` blocks are the accounts on that service. `password_wo` is sent when the user is created and when `password_wo_version` changes. Terraform stores the version, not the password. `vergeio_nas_volume` is a volume on that service. Destroy disables the volume and waits until VergeOS allows the delete. `vergeio_nas_cifs_share` and `vergeio_nas_nfs_share` are shares on a volume. Volume snapshots and volume sync are not part of these resources.

`vergeio_vm_recipe_instance` deploys a VM from a catalog recipe. `vergeio_catalogs` and `vergeio_vm_recipes` look the recipe up by name. `answers` is a map of strings. A disk size is bytes, so 50 GB is `53687091200`. A bool answer is `true`, `false`, `yes`, `no`, `on`, `off`, `1`, or `0`. The resource deploys the VM. It has no simulate argument.

`vergeio_group` creates a group. `vergeio_permission` grants a user or a group list, read, create, modify, and delete on a table or one object. `vergeio_member` adds a user or another object to a group. `vergeio_users` lists users the same way `vergeio_groups` lists groups.

`vergeio_api_key` manages a long-lived user API key. It sets the name, description, expiry, and IP allow and deny lists. VergeOS returns the bearer token only when the key is created, and the resource does not store it. The ephemeral `vergeio_api_key` above is the short-lived token for one run. Do not reuse a name across the two.

`vergeio_auth_source` manages an external identity provider. `settings` is the non-secret JSON document. `client_secret_wo` is sent on create and when `client_secret_wo_version` changes. Terraform stores the version, not the secret. An update merges settings into the stored document so a partial change does not wipe the client secret. A key removed from `settings` stays on the auth source. Replace the auth source to drop it. Changing `driver` replaces the auth source.

`vergeio_snapshot_profile` defines when snapshots are taken and how long they are kept. Each `period` block sets the frequency, the time of day, a required retention in seconds, and whether the snapshot is quiesced. Retention has no default. `vergeio_vm.snapshot_profile` is that profile's key: `snapshot_profile = tonumber(vergeio_snapshot_profile.example.id)`.

`vergeio_tag_category` creates a tag category and chooses which object types can use it. `vergeio_tag` creates a tag in that category. Deleting a category deletes every tag in it and every assignment of those tags, with no confirmation from VergeOS. Omit a `taggable_*` flag to leave that object type unchanged; an omitted flag is not sent as false.

`vergeio_tenant` creates a tenant on the parent system, with its power state and UI address. `vergeio_tenant_node` and `vergeio_tenant_storage` hand that tenant compute and storage. `vergeio_tenant_external_ip` assigns one virtual IP on a parent network to the tenant. The first assigned IP becomes the UI address. Set `ui_address_id` on `vergeio_tenant` to choose a different assigned IP. A parent UI move of that value is drift, and the next apply restores it. `vergeio_tenant_network_block` assigns one routed CIDR from a parent network to the tenant. `vergeio_tenant_layer2_network` bridges one parent layer 2 network into the tenant on VergeOS 26.0 or later. Destroy disables that assignment, then deletes it. Networks created inside the tenant remain after the host-side delete and belong to the tenant-side configuration. A second Terraform configuration, pointed at `ui_address`, manages the inside of the tenant. The tenants guide in the docs has a working parent stack and tenant stack.

Assigning an external IP or a network block can leave `need_fw_apply` set on the parent network. Both resources report that flag as `parent_firewall_pending`. Set `apply_parent_firewall` to apply the rules in the same call. VergeOS refuses to delete a network block while a tenant network is still built on it.

## Data Sources

- vergeio_catalogs
- vergeio_cloudinit_files
- vergeio_clusters
- vergeio_groups
- vergeio_mediasources
- vergeio_networks
- vergeio_nodes
- vergeio_resource_groups
- vergeio_tags
- vergeio_tenants
- vergeio_users
- vergeio_version
- vergeio_vm_recipes
- vergeio_vms

# Building Provider From Source

**Prerequisites:**

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.10, or OpenTofu >= 1.11 for the same features. Ephemeral `vergeio_api_key` needs Terraform 1.10 or OpenTofu 1.11. Write-only attributes need Terraform 1.11 or OpenTofu 1.11. OpenTofu 1.10 and earlier reject both. Actions need Terraform 1.14. OpenTofu does not implement actions.
- [Go](https://golang.org/doc/install) >= 1.27

```
go install .
```

Point Terraform at that binary with a `dev_overrides` entry for `vergeio/cloud/vergeio`. The directory is the Go bin path, not a hand-built plugin cache. `CONTRIBUTING.md` has the CLI configuration and the GoReleaser release flow.

### Test sample configuration

Create a main tf file in a workspace directory using the example below

```
terraform {
	required_providers {
		vergeio = {
			source  = "vergeio/cloud/vergeio"
		}
	}
}

provider "vergeio" {
	host = "Hostname_or_IP"
	username = "username"
	password = "password"
	insecure = false
	# api_key = "my_api_key" # used instead of username and password
	# timeout = 60           # seconds; VERGEOS_TIMEOUT when omitted
}

resource "vergeio_vm" "new_vm" {
	name  = "NEW VM"
	description = "NEW TF VM"
	enabled = true
	os_family = "linux"
	cpu_cores = 4
	machine_type = "q35"
	ram = 8192
}
```

Within the workspace run ` terraform init && terraform apply`
