# Pass one host device to a tenant node.
# resource_group is the resource group UUID from vergeio_resource_groups.
# The tenant owns the device once it is attached, even when no VM inside
# the tenant is using it. It cannot be assigned to another VM or tenant
# until this resource is destroyed.
# device_count is sent as count. It defaults to 1. Every copy is listed in
# device_ids, and destroy removes all of them.

data "vergeio_resource_groups" "passthrough" {
  filter_name = "GPU"
}

resource "vergeio_tenant_node_device" "gpu" {
  tenant_node_id = vergeio_tenant_node.example.id
  name           = "gpu0"
  description    = "GPU for the tenant"
  type           = data.vergeio_resource_groups.passthrough.resource_groups[0].type
  resource_group = data.vergeio_resource_groups.passthrough.resource_groups[0].id
  device_count   = 1
  optional       = false
  enabled        = true
}
