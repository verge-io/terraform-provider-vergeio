# contact is the user id whose email Let's Encrypt uses.
# agree_tos must be true.

resource "vergeio_certificate" "acme" {
  type        = "letsencrypt"
  domain_name = "ui.example.com"
  domain_list = "ui.example.com,api.example.com"
  contact     = 1
  agree_tos   = true
}
