# Tags data source fetches the Tags from the VergeIO.

data "vergeio_tags" "all" {
}
output "tags" {
  value = data.vergeio_tags.all.tags
}

# Add a filter to see information on a specific tag

data "vergeio_tags" "production" {
  filter = "production"
}
output "production_tags" {
  value = data.vergeio_tags.production.tags
}