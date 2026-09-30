data "vergeio_tenants" "all" {
}

data "vergeio_tenants" "customer" {
  filter_name = "customer-a"
}

output "tenant_ui_address" {
  value = data.vergeio_tenants.customer.tenants[0].ui_address
}
