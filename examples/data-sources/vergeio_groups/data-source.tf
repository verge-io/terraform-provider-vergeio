
# Groups data source fetches the Groups from the VergeIO. 

data "vergeio_groups" "all" {
}
output "vergeio_groups_id" {
  value = data.vergeio_groups.all
}

# Add a filter to see information on a specific group

data "vergeio_groups" "administrators" {
  filter_name = "Administrators (default)"
}
output "vergeio_groups_administrators" {
  value = data.vergeio_groups.administrators
}
