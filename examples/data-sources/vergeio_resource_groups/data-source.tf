# Get all resource groups
data "vergeio_resource_groups" "all" {
}

output "resource_groups" {
  value = data.vergeio_resource_groups.all.resource_groups
}

# Filter by name
data "vergeio_resource_groups" "gpu" {
  filter_name = "GPU Pool"
}

output "gpu_groups" {
  value = data.vergeio_resource_groups.gpu.resource_groups
}
