terraform {
	required_providers {
		vergeio = {
	  		source  = "vergeio/cloud/vergeio"
		}
  	}
}

data "vergeio_clusters" "all" {
	#filter_name = "BGill"
}

output "cluster_id" {
	
	value = data.vergeio_clusters.all.clusters
}
