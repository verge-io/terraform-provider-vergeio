# settings is the non-secret JSON document. client_secret_wo is not stored.
# Increment client_secret_wo_version to send a new secret.
# An update merges settings, so a key removed here stays on the auth source.
# Replace the auth source to drop a settings key. Changing driver replaces it.

resource "vergeio_auth_source" "azure" {
  name   = "Corporate Azure"
  driver = "azure"

  settings = jsonencode({
    tenant_id         = "00000000-0000-0000-0000-000000000000"
    client_id         = "app-registration-id"
    scope             = "openid profile email"
    update_user_email = true
  })

  client_secret_wo         = "change-me"
  client_secret_wo_version = 1
  button_fa_icon           = "bi-microsoft"
}
