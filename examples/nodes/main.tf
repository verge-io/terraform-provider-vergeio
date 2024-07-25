terraform {
	required_providers {
		vergeio = {
	  		source  = "vergeio/cloud/vergeio"
		}
  	}
}

variable "image_id" {
  type = string
}

data "vergeio_nodes" "all" {}
data "vergeio_nodes" "nodedata" {
	filter_name = "node1"
}

output "node1_id" {
	value = data.vergeio_nodes.nodedata.nodes[0].id
}
