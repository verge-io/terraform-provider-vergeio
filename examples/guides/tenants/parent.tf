provider "vergeio" {
  host     = "parent.example.com"
  username = "admin"
  password = var.parent_password
}

variable "parent_password" {
  type      = string
  sensitive = true
}

variable "tenant_password" {
  type      = string
  sensitive = true
}

resource "vergeio_tenant" "customer" {
  name        = "customer-a"
  description = "Customer A virtual data center"
  password    = var.tenant_password
  powerstate  = true
}

resource "vergeio_tenant_node" "node" {
  tenant_id = vergeio_tenant.customer.id
  name      = "node1"
  cpu_cores = 4
  ram       = 8192
  enabled   = true
}

resource "vergeio_tenant_storage" "tier" {
  tenant_id   = vergeio_tenant.customer.id
  tier        = 1
  provisioned = 107374182400
}

variable "parent_external_network_id" {
  type        = string
  description = "Key of the parent external network that hands the tenant its UI address and routed blocks."
}

# The first assigned IP becomes the tenant UI address.
# Set vergeio_tenant.ui_address_id to this resource's id on a later change
# when a different assigned IP should be the UI address.
resource "vergeio_tenant_external_ip" "ui" {
  tenant_id             = vergeio_tenant.customer.id
  network_id            = var.parent_external_network_id
  ip                    = "203.0.113.50"
  hostname              = "customer-a"
  description           = "Tenant UI address"
  apply_parent_firewall = true
}

resource "vergeio_tenant_network_block" "routed" {
  tenant_id             = vergeio_tenant.customer.id
  network_id            = var.parent_external_network_id
  cidr                  = "198.51.100.0/28"
  description           = "Customer A routed addresses"
  apply_parent_firewall = true
}

variable "parent_layer2_network_id" {
  type        = string
  description = "Key of the parent layer 2 network bridged into the tenant. Requires VergeOS 26.0 or later."
}

resource "vergeio_tenant_layer2_network" "external" {
  tenant_id  = vergeio_tenant.customer.id
  network_id = var.parent_layer2_network_id
  enabled    = true
}

output "tenant_ui_address" {
  value = vergeio_tenant.customer.ui_address
}

output "tenant_id" {
  value = vergeio_tenant.customer.id
}
