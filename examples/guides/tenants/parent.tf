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
  description = "Key of the parent external network that hands the tenant its UI address."
}

resource "vergeio_tenant_external_ip" "ui" {
  tenant_id             = vergeio_tenant.customer.id
  network_id            = var.parent_external_network_id
  ip                    = "203.0.113.50"
  hostname              = "customer-a"
  description           = "Tenant UI address"
  apply_parent_firewall = true
}

output "tenant_ui_address" {
  value = vergeio_tenant.customer.ui_address
}

output "tenant_id" {
  value = vergeio_tenant.customer.id
}
