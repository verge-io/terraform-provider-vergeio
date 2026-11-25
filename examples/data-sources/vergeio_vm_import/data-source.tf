# Wait for an import that Ansible or the UI already started.
# Omit wait to block until the row is complete or warning.

data "vergeio_vm_import" "example" {
  id = "8f73f8bcc9c9f1aaba32f733bfc295acaf548554"
}

output "imported_vm" {
  value = data.vergeio_vm_import.example.vm_id
}
