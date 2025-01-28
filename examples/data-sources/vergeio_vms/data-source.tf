
# VMs data source fetches the VMsfrom the VergeIO. 

data "vergeio_vms" "all" {
}
output "Virtual_Machines" {
  value = data.vergeio_vms.all.vms
}

# Add a filter to see information on a specific virtual machine or ignore VM snapshots

data "vergeio_vms" "all" {
  filter_name = "Example VM"
  is_snapshot = false
}
output "Virtual_Machines" {
  value = data.vergeio_vms.all.vms
}
