# A certificate VergeOS generates for the UI.
# Changing type or domain_name replaces the certificate.

resource "vergeio_certificate" "example" {
  type        = "self_signed"
  domain_name = "ui.example.com"
  key_type    = "ecdsa"
  description = "UI certificate"
}
