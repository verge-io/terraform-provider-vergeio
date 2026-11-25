terraform {
  required_version = ">= 1.11.0"

  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

# id is unknown until apply. length of that id is an unknown number with a
# static map key, which is how a tag created in the same configuration is passed.
resource "terraform_data" "tag" {}

module "vm" {
  source = "../.."

  name            = "web1"
  source_type     = "image"
  media_source_id = 25
  tag_ids = {
    web = length(terraform_data.tag.id)
  }
}

output "tag_member_keys" {
  value = keys(module.vm.tag_member_ids)
}
