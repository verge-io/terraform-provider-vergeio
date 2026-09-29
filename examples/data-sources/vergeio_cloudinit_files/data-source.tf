# Get all cloud-init files
data "vergeio_cloudinit_files" "all" {
}

output "cloudinit_files" {
  value = data.vergeio_cloudinit_files.all.cloudinit_files
}

# Filter by name
data "vergeio_cloudinit_files" "userdata" {
  filter_name = "user-data"
}

output "userdata_files" {
  value = data.vergeio_cloudinit_files.userdata.cloudinit_files
}
