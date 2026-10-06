data "vergeio_vm_recipes" "windows" {
  catalog_name = "Operating Systems"
  filter_name  = "Windows Server"
}

output "windows_recipe_id" {
  value = data.vergeio_vm_recipes.windows.recipes[0].id
}
