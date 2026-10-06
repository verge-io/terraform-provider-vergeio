# Bootstrap credentials come from the environment, not this configuration:
#   VERGEOS_USERNAME and VERGEOS_PASSWORD sign in as the tenant admin.
# Those values are not written to state. Provider arguments cannot be
# write-only, so a password or api_key set in a provider block is stored.
# The ephemeral token is not stored either. Close deletes the key.

data "terraform_remote_state" "parent" {
  backend = "local"
  config = {
    path = "../parent/terraform.tfstate"
  }
}

provider "vergeio" {
  alias    = "bootstrap"
  host     = data.terraform_remote_state.parent.outputs.tenant_ui_address
  insecure = true
}

data "vergeio_users" "admin" {
  provider    = vergeio.bootstrap
  filter_name = "admin"
}

# name is exclusive. Open deletes an existing key with this name for
# user_id, then creates a new one. Do not reuse a long-lived key name.
ephemeral "vergeio_api_key" "run" {
  provider    = vergeio.bootstrap
  user_id     = data.vergeio_users.admin.users[0].id
  name        = "terraform-tenant"
  ttl_seconds = 3600
}

provider "vergeio" {
  host     = data.terraform_remote_state.parent.outputs.tenant_ui_address
  api_key  = ephemeral.vergeio_api_key.run.token
  insecure = true
}

resource "vergeio_vm" "app" {
  name      = "app"
  os_family = "linux"
  cpu_cores = 2
  ram       = 2048
}
