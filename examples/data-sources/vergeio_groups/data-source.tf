


# Groups data source fetches the Groups from the VergeIO. 

data "vergeio_groups" "all" {
}
output "vergeio_groups_id" {
  value = data.vergeio_groups.all
}

# Add a fliter to see information on a specific group

data "vergeio_groups" "all" {
  filter_name = "Administrators (default)"
}
output "vergeio_groups_id" {
  value = data.vergeio_groups.all
}
