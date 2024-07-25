terraform {
	required_providers {
		vergeio = {
			source  = "vergeio/cloud/vergeio"
		}
	}
}

resource "vergeio_network" "terranet" {
	name  = "terranet"
	enabled = true
	vnet_default_gateway = 3
	ipaddress = "10.255.252.254/24"
}