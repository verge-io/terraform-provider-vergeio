# Deleting the category deletes this tag and every assignment of it.
# VergeOS does not ask for confirmation.

resource "vergeio_tag_category" "environment" {
  name                 = "environment"
  description          = "Environment classification"
  single_tag_selection = true
  taggable_vms         = true
  taggable_vnets       = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "vergeio_tag" "production" {
  category    = tonumber(vergeio_tag_category.environment.id)
  name        = "production"
  description = "Production workloads"
}

resource "vergeio_tag_member" "web" {
  tag_id = tonumber(vergeio_tag.production.id)
  member = "vms/54"
}
