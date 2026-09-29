# category_name selects the category when the id is not known.
# It cannot be combined with category_filter.

data "vergeio_tags" "powerup_true" {
  filter        = "true"
  category_name = "powerup"
}

data "vergeio_tags" "all_backup_tags" {
  category_name = "backup"
}
