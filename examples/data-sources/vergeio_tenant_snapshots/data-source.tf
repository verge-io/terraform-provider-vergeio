data "vergeio_tenant_snapshots" "customer" {
  tenant_id = vergeio_tenant.example.id
}

output "snapshot_names" {
  value = [for snap in data.vergeio_tenant_snapshots.customer.snapshots : snap.name]
}
