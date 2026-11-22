
# Nodes data source fetches the Nodes from the VergeIO. 

data "vergeio_nodes" "all" {
}
output "nodes" {
  value = data.vergeio_nodes.all
}

# Add a filter to see information on a specific node

data "vergeio_nodes" "node1" {
  filter_name = "node1"
}
output "node1" {
  value = data.vergeio_nodes.node1
}
