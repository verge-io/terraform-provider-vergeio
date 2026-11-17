# private_key_wo is sent on create and when private_key_wo_version changes.
# Terraform stores the version, not the private key.

resource "vergeio_certificate" "uploaded" {
  type        = "manual"
  domain_name = "ui.example.com"
  description = "Uploaded UI certificate"

  public_certificate = <<-EOT
  -----BEGIN CERTIFICATE-----
  MIIB
  -----END CERTIFICATE-----
  EOT

  private_key_wo = <<-EOT
  -----BEGIN PRIVATE KEY-----
  MIIB
  -----END PRIVATE KEY-----
  EOT

  private_key_wo_version = 1
}
