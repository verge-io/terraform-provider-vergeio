


# Clusters data source fetches the Clusters from the VergeIO. 

data "vergeio_clusters" "all" {
}
output "cluster_id" {
  value = data.vergeio_clusters.all.clusters
}

# Add a fliter to see information on a specific cluster

data "vergeio_clusters" "all" {
  filter_name = "Compute"
}
output "cluster_id" {
  value = data.vergeio_clusters.all.clusters
}
