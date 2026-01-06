# Create a tag member assignment to assign a tag to an object

resource "vergeio_tag_member" "vm_production" {
  tag_id = 1
  member = "vms/54"
}

# Assign tag to a network
resource "vergeio_tag_member" "network_production" {
  tag_id = 2
  member = "vnets/19"
}

# Use with other resources - assign tag to a newly created VM
resource "vergeio_vm" "web-server" {
  name         = "example-web-server"
  cpu_cores    = 2
  ram          = 2048
  machine_type = "q35"
  os_family    = "linux"
}

resource "vergeio_tag_member" "web_server_tag" {
  tag_id = 1
  member = "vms/${vergeio_vm.web-server.id}"
}