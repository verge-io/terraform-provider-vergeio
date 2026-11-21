# Bootstrap credentials come from the environment (VERGEOS_USERNAME and
# VERGEOS_PASSWORD) or a bootstrap provider configuration. The data source
# and ephemeral resource use that bootstrap provider. The second provider
# configuration then consumes the minted ephemeral token.
#
# Mints a key for this run. Close deletes it. ttl_seconds expires it
# if Close does not run. name is exclusive: Open deletes an existing
# key with this name for user_id before creating a new one.

terraform {
  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "vergeio" {
  alias = "bootstrap"
  host  = "tenant.example.com"
}

data "vergeio_users" "admin" {
  provider    = vergeio.bootstrap
  filter_name = "admin"
}

ephemeral "vergeio_api_key" "run" {
  provider    = vergeio.bootstrap
  user_id     = data.vergeio_users.admin.users[0].id
  name        = "terraform-tenant"
  description = "Short-lived key for this Terraform run"
  ttl_seconds = 3600
}

provider "vergeio" {
  host    = "tenant.example.com"
  api_key = ephemeral.vergeio_api_key.run.token
}
