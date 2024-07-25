terraform {
	required_providers {
		vergeio = {
	  		source  = "vergeio/cloud/vergeio"
		}
  	}
}

data "vergeio_authsources" "all" {
	filter_driver = "google"
}

output "authsources" {
  value = data.vergeio_authsources.all
}
