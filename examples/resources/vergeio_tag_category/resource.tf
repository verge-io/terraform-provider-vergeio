# Deleting this category deletes every tag in it and every assignment of
# those tags. VergeOS does not ask for confirmation. prevent_destroy blocks
# an accidental destroy. Remove it only when that cascade is intentional.
#
# Only the taggable_* flags set here are sent. Leave the others unset so an
# update does not turn tagging off for those object types.

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
