# Separate example from image.tf. Provider 3.0 reads VERGEOS_HOST and either
# VERGEOS_API_KEY or VERGEOS_USERNAME plus VERGEOS_PASSWORD.

provider "vergeio" {}

data "vergeio_clusters" "compute" {
  filter_name = "Compute"
}

resource "vergeio_tag_category" "role" {
  name                 = "role"
  description          = "Workload role"
  single_tag_selection = true
  taggable_vms         = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "vergeio_tag" "web" {
  category    = tonumber(vergeio_tag_category.role.id)
  name        = "web"
  description = "Web servers"
}

resource "vergeio_vm" "web" {
  name      = "web1"
  os_family = "linux"
  cpu_cores = 2
  ram       = 2048
  cluster   = data.vergeio_clusters.compute.clusters[0].id
}

resource "vergeio_tag_member" "web" {
  tag_id = tonumber(vergeio_tag.web.id)
  member = "vms/${vergeio_vm.web.id}"
}
