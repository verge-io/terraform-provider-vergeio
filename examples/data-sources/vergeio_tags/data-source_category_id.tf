# The same tag name can exist in more than one category. category_filter
# selects the category by id.

data "vergeio_tags" "powerup_true" {
  filter          = "true"
  category_filter = 5
}

data "vergeio_tags" "backup_true" {
  filter          = "true"
  category_filter = 8
}
