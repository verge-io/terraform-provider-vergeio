data "vergeio_catalogs" "operating_systems" {
  filter_name = "Operating Systems"
}

data "vergeio_vm_recipes" "windows" {
  catalog_id  = data.vergeio_catalogs.operating_systems.catalogs[0].id
  filter_name = "Windows Server"
}

# answers values are strings. A disksize question is bytes: 50 GB is
# 53687091200. 50 is fifty bytes and is rejected. A bool question accepts
# true, false, yes, no, on, off, 1, or 0. enabled is rejected.
resource "vergeio_vm_recipe_instance" "windows" {
  name      = "windows-01"
  recipe_id = data.vergeio_vm_recipes.windows.recipes[0].id

  answers = {
    HOSTNAME           = "windows-01"
    SELECT_CREATE_UEFI = "true"
    YB_DRIVE_OS_SIZE   = "53687091200"
    YB_NIC_ETH0        = "Internal"
  }
}
