---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "vergeio_nic Resource - terraform-provider-vergeio"
subcategory: ""
description: |-
  
---

# vergeio_nic (Resource)

Add a NIC to a Virtual Machine

# Example Usage
```
data "vergeio_vms" "example_vm" {
    filter_name = "Example VM"
}
output "Virtual_Machines" {
	value = data.vergeio_vms.example_vm.vms
}
data "vergeio_networks" "example_network" {
    filter_name="example_network"
}
output "networks" {
	value = data.vergeio_networks.example_network.networks
}
resource "vergeio_nic" "example_nic" {
    machine = data.vergeio_vms.example_vm.vms[0].id
    name = "Example NIC 1"
    description = "Example NIC"
    vnet = data.vergeio_networks.example_network.networks[0].id
    enabled = true
    interface = "virtio"
}
```
<!-- schema generated by tfplugindocs -->
## Arguments

### Required

- `machine` (Number) - ID of the virtual machine the resource will attach to.
- `name` (String)

### Optional

- `description` (String)
- `enabled` (Boolean) - Default = True
- `interface` (String)
  - `virtio`  (Virtio, **Default**)
  - `e1000`   (Intel)
  - `rtl8139` (Realtek 8139)
  - `pcnet`   (AMD PCNET)
- `macaddress` (String)
- `vnet` (Number) - Key (ID) of the vNET the resource will attach to.

### Read-Only

- `id` (String) - ID of this resource.
