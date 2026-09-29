# Look up a tag by name, then assign it.

data "vergeio_tags" "production" {
  filter = "production"
}

resource "vergeio_tag_member" "vm_tag" {
  tag_id = data.vergeio_tags.production.tags[0].key
  member = "vms/123"
}
