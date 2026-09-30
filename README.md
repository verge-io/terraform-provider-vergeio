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

## Resources

- vergeio_group
- vergeio_member
- vergeio_network
- vergeio_network_rule
- vergeio_network_rule_alias
- vergeio_network_rules
- vergeio_permission
- vergeio_tag
- vergeio_tag_category
- vergeio_tag_member
- vergeio_tenant
- vergeio_tenant_node
- vergeio_tenant_storage
- vergeio_user
- vergeio_vm

Drives and NICs are `vergeio_vm_drive` and `vergeio_vm_nic`. Devices stay nested on `vergeio_vm`.

`vergeio_group` creates a group. `vergeio_permission` grants a user or a group list, read, create, modify, and delete on a table or one object. `vergeio_member` adds a user or another object to a group. `vergeio_users` lists users the same way `vergeio_groups` lists groups.

`vergeio_tag_category` creates a tag category and chooses which object types can use it. `vergeio_tag` creates a tag in that category. Deleting a category deletes every tag in it and every assignment of those tags, with no confirmation from VergeOS. Omit a `taggable_*` flag to leave that object type unchanged; an omitted flag is not sent as false.

`vergeio_tenant` creates a tenant on the parent system, with its power state and UI address. `vergeio_tenant_node` and `vergeio_tenant_storage` hand that tenant compute and storage. A second Terraform configuration, pointed at `ui_address`, manages the inside of the tenant. The tenants guide in the docs has a working parent stack and tenant stack.

Network blocks and external IPs are not resources yet. govergeos has no `vnet_cidrs` service and no helper that assigns an external IP to a tenant. Assigning those in the UI can leave `need_fw_apply` set on the parent external network. Apply that network's firewall before treating the address as live.

## Data Sources

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
- vergeio_vms

# Building Provider From Source

**Prerequisites:**

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.10
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
