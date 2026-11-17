data "vergeio_catalogs" "all" {}

data "vergeio_catalogs" "operating_systems" {
  filter_name = "Operating Systems"
}

output "catalog_id" {
  value = data.vergeio_catalogs.operating_systems.catalogs[0].id
}
