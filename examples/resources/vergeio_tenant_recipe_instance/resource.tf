data "vergeio_catalogs" "tenants" {
  filter_name = "Tenants"
}

data "vergeio_tenant_recipes" "trial" {
  catalog_id  = data.vergeio_catalogs.tenants.catalogs[0].id
  filter_name = "30-Day Trial (POC)"
}

# answers values are strings. A disksize question is bytes: 50 GB is
# 53687091200. 50 is fifty bytes and is rejected. A bool question accepts
# true, false, yes, no, on, off, 1, or 0. enabled is rejected.
# tenant_id is the new tenant's key. Child resources take that key as a string.
resource "vergeio_tenant_recipe_instance" "customer" {
  name      = "customer-a"
  recipe_id = data.vergeio_tenant_recipes.trial.recipes[0].id

  answers = {
    YB_USER_NAME              = "admin"
    YB_EXPOSE_CLOUD_SNAPSHOTS = "true"
    YB_DRIVE_OS_SIZE          = "53687091200"
    YB_NIC_ETH0               = "Internal"
  }
}

resource "vergeio_tenant_node" "node" {
  tenant_id = tostring(vergeio_tenant_recipe_instance.customer.tenant_id)
  name      = "node1"
  cpu_cores = 4
  ram       = 8192
}
