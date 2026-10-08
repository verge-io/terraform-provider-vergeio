data "vergeio_tenant_recipes" "trial" {
  catalog_name = "Tenants"
  filter_name  = "30-Day Trial (POC)"
}

output "trial_recipe_id" {
  value = data.vergeio_tenant_recipes.trial.recipes[0].id
}
