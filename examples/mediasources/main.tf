terraform {
	required_providers {
		vergeio = {
	  		source  = "vergeio/cloud/vergeio"
		}
  	}
}

data "vergeio_mediasources" "all" {
	filter_name = "verge.io-clone.iso"
}

output "cloneiso" {
	value = data.vergeio_mediasources.all.mediasources
}

