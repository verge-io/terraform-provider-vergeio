# A certificate VergeOS generates for the UI.
# Changing type replaces the certificate. Changing this domain_name replaces it too.

resource "vergeio_certificate" "example" {
  type        = "self_signed"
  domain_name = "ui.example.com"
  key_type    = "ecdsa"
  description = "UI certificate"
}
