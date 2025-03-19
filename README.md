# Terraform VergeIO Provider

Terraform provider plugin to integrate with VergeOS

## Support

VergeIO welcomes pull requests and responds to issues on a best-effort basis. VergeIO maintains public GitHub repositories for initiatives that help customers integrate the VergeIO platform with other third-party products. Support for these initiatives is handled directly via the GitHub repository. Issues and enhancement requests can be submitted in the Issues tab of each repository. Search for and review existing open issues before submitting a new issue.

## Example Usage

See the docs folder for examples

## Configuration Reference

- **host** - (**Required**) URL or IP address for the system or tenant.
- **username** - (**Required**) Username for the system or tenant.
- **password** - (**Required**) Password for the provided username.
- **insecure** (**Optional**) Required for systems with self-signed SSL certificates

```
provider "vergeio" {
	host = "Hostname_or_ip"
	username = "my_user"
	password = "my_password"
	insecure = false
}
```

## Resources

- vergeio_drive
- vergeio_member
- vergeio_network
- vergeio_nic
- vergeio_user
- vergeio_vm

## Data Sources

- vergeio_clusters
- vergeio_groups
- vergeio_mediasources
- vergeio_networks
- vergeio_nodes
- vergeio_version
- vergeio_vms

# Building Provider From Source

**Prerequisites:**

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.10
- [Go](https://golang.org/doc/install) >= 1.23

**Note for ARM devices:** If you are building on an ARM-based system (like Apple M1/M2 or ARM Linux), you'll need to modify line 7 in the Makefile. Change `GOARCH=linux_amd64` to `GOARCH=darwin_arm64` before running `make install`.

1. Clone the repository
2. Enter the repository directory
3. Build the provider

```
# Build the provider
go build -o terraform-provider-vergeio

# Install the provider
make install
```

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
