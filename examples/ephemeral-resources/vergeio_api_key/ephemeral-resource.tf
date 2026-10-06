# Mints a key for this run. Close deletes it. ttl_seconds expires it
# if Close does not run. name is exclusive: Open deletes an existing
# key with this name for user_id before creating a new one.

data "vergeio_users" "admin" {
  filter_name = "admin"
}

ephemeral "vergeio_api_key" "run" {
  user_id     = data.vergeio_users.admin.users[0].id
  name        = "terraform-tenant"
  description = "Short-lived key for this Terraform run"
  ttl_seconds = 3600
}

provider "vergeio" {
  host    = "tenant.example.com"
  api_key = ephemeral.vergeio_api_key.run.token
}
