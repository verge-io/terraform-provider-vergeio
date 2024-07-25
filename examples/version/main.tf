terraform {
	required_providers {
		vergeio = {
	  		source  = "vergeio/cloud/vergeio"
		}
  	}
}

data "vergeio_version" "all" {}

output "version" {
  value = data.vergeio_version.all
}
