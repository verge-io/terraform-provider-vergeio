# A destination for event notifications.
# authorization_value_wo is sent on create and when the version changes.
# Terraform stores the version, not the credential.

resource "vergeio_webhook_url" "example" {
  name    = "Chat alerts"
  url     = "https://example.com/hooks/chat"
  timeout = 10
  retries = 3

  authorization_type             = "bearer"
  authorization_value_wo         = "example-token"
  authorization_value_wo_version = 1
}
