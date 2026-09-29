data "vergeio_tags" "all" {}

output "tags_with_categories" {
  value = [for tag in data.vergeio_tags.all.tags : {
    id            = tag.key
    name          = tag.name
    category_id   = tag.category
    category_name = tag.category_name
  }]
}
