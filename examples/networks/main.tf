terraform {
	required_providers {
		vergeio = {
			source  = "vergeio/cloud/vergeio"
		}
	}
}

data "vergeio_networks" "all" {}

output "networks" {
	value = data.vergeio_networks.all.networks
}
