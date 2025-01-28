
# Version data source fetches the API Version from the VergeIO. 

data "vergeio_version" "all" {
}
output "version" {
  value = data.vergeio_version.all
}
