
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

# VMs and NICs information is also available via the VMs data source

output "VM_NICs" {
  value = data.vergeio_vms.all.vms[0].nics[0]
}

output "VM_Drives" {
  value = data.vergeio_vms.all.vms[0].drives[0]
}
