# value_wo is for a setting whose value should not be stored.
# Terraform stores value_wo_version, not the secret.
# max_connections is a real key. Use value_wo on a key that holds a secret.

resource "vergeio_setting" "secret_example" {
  key              = "max_connections"
  value_wo         = "200"
  value_wo_version = 1
}
