


# Mediasources data source fetches the Media Source from the VergeIO. 

data "vergeio_mediasources" "all" {
}
output "Available_Media_Images" {
  value = data.vergeio_mediasources.all.mediasources
}

# Add a filter to view details about a specific image

data "vergeio_mediasources" "all" {
  filter_name = "verge.io-clone.iso"
}
output "cloneiso" {
  value = data.vergeio_mediasources.all.mediasources
}
