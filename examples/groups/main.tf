terraform {
	required_providers {
		vergeio = {
	  		source  = "vergeio/cloud/vergeio"
		}
  	}
}

data "vergeio_groups" "all" {}
data "vergeio_groups" "beagle" { 
	filter_name = "beagle groop"
}
output "beagle_group_id" {
	value = data.vergeio_groups.beagle.groups[0].id
}

