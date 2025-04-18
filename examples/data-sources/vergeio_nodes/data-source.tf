
# Networks data source fetches the Network from the VergeIO. 

data "vergeio_nodes" "all" {
}
output "nodes" {
  value = data.vergeio_nodes.all
}

# Add a filter to see information on a specific node

data "vergeio_nodes" "all" {
  filter_name = "node1"
}
output "nodes" {
  value = data.vergeio_nodes.all
}
