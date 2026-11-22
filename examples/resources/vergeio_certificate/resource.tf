# A certificate VergeOS generates for the UI.
# Changing type or domain_name replaces the certificate.
# Changing domain_list, key_type, or rsa_key_size replaces this self_signed certificate.

resource "vergeio_certificate" "example" {
  type        = "self_signed"
  domain_name = "ui.example.com"
  key_type    = "ecdsa"
  description = "UI certificate"
}
