data "vergeio_tenant_node_devices" "node" {
  tenant_node_id = vergeio_tenant_node.example.id
}

output "device_names" {
  value = [for device in data.vergeio_tenant_node_devices.node.devices : device.name]
}
