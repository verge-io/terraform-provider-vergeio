# A long-lived key for one user. VergeOS returns the bearer token only
# when the key is created, and this resource does not store it.
# Use ephemeral.vergeio_api_key when a run needs a token.
# Do not reuse this name with that ephemeral resource: Open deletes
# an existing key of the same name for the user.

resource "vergeio_user" "automation" {
  name            = "automation"
  displayname     = "Automation"
  email           = "automation@example.com"
  enabled         = true
  type            = "normal"
  password_wo         = "change-me"
  password_wo_version = 1
}

resource "vergeio_api_key" "ci" {
  user_id       = tonumber(vergeio_user.automation.id)
  name          = "ci"
  description   = "CI runner"
  ip_allow_list = "192.0.2.0/24"
  ip_deny_list  = "198.51.100.10"
  expires       = 1893456000
}
