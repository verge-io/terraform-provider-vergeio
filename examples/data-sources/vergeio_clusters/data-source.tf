
# Clusters data source fetches the Clusters from the VergeIO. 

data "vergeio_clusters" "all" {
}
output "cluster_id" {
  value = data.vergeio_clusters.all.clusters
}

# Add a filter to see information on a specific cluster

data "vergeio_clusters" "compute" {
  filter_name = "Compute"
}
output "compute_cluster" {
  value = data.vergeio_clusters.compute.clusters
}
