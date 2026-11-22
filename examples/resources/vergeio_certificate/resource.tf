# A certificate VergeOS generates for the UI.
# Changing type, domain_name, or domain_list replaces the certificate.
# A self_signed certificate is always RSA 2048.

resource "vergeio_certificate" "example" {
  type        = "self_signed"
  domain_name = "ui.example.com"
  description = "UI certificate"
}
