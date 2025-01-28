
# Networks data source fetches the Network from the VergeIO. 

data "vergeio_networks" "all" {
}
output "networks" {
  value = data.vergeio_networks.all.networks
}

# Add a filter to see information on a specific network

data "vergeio_networks" "all" {
  filter_name = "External"
}
output "networks" {
  value = data.vergeio_networks.all.networks
}
