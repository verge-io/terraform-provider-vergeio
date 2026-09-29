# Filter by network type (internal or external).

data "vergeio_networks" "internal_only" {
  filter_type = "internal"
}

output "internal_networks" {
  value = data.vergeio_networks.internal_only.networks
}
