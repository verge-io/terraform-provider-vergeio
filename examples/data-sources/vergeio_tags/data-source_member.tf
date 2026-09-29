data "vergeio_tags" "powerup_enabled" {
  filter        = "true"
  category_name = "powerup"
}

resource "vergeio_tag_member" "vm_powerup" {
  tag_id = data.vergeio_tags.powerup_enabled.tags[0].key
  member = "vms/123"
}
