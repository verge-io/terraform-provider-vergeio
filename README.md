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
	host = "https://some_url_or_ip"
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

## Building Provider From Source
Run the following to build and install the provider

```
- go build -o terraform-provider-vergeio
- make install
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
	host = "https://someURLorIP"
	username = "username"
	password = "password"
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
Within the workspace run ``` terraform init && terraform apply```
