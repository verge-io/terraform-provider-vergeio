# Users data source fetches users from VergeOS.

data "vergeio_users" "all" {
}

output "vergeio_users" {
  value = data.vergeio_users.all
}

# Filter to one user name.

data "vergeio_users" "admin" {
  filter_name = "admin"
}

output "vergeio_admin_user" {
  value = data.vergeio_users.admin
}
