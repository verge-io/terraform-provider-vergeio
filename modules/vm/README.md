# vm

Creates a VM from a media image or a catalog recipe, with cloud-init, Ansible inventory tags, and an optional snapshot profile.

`source_type` `image` creates `vergeio_vm`. The boot disk uses `media` `import` and `source` set to `media_source_id` (`vergeio_mediasources` id). `cloudinit_user_data` writes a `user-data` file and a `meta-data` file. Both bodies are stored in state. Set `cloudinit_meta_data` to replace the generated instance id and hostname. The provider rejects an unknown `cloudinit_files` list, so this module passes a fixed two-file list instead of an arbitrary list. NICs are `vergeio_vm_nic`. `vnet` is the network key as a number.

`source_type` `recipe` creates `vergeio_vm_recipe_instance`. `recipe_id` is the 40-character key from `vergeio_vm_recipes`. Guest hostname, disk size, and network answers go in `recipe_answers`. That resource has no `cloudinit_files`, NIC, or `snapshot_profile` attribute, so those inputs stay on image VMs.

`tags` creates a category with `taggable_vms`, a tag, and a `vergeio_tag_member` of `vms/<id>`. Deleting the category deletes its tags and every assignment of those tags. `tag_ids` assigns tags that already exist.

`snapshot_profile` creates a `vergeio_snapshot_profile` and sets `vergeio_vm.snapshot_profile` to its key. `retention` is seconds and is required on every period. `snapshot_profile_id` attaches a profile that already exists.

```hcl
module "web" {
  source = "../../modules/vm"

  name            = "web1"
  source_type     = "image"
  media_source_id = data.vergeio_mediasources.golden.mediasources[0].id
  cpu_cores       = 2
  ram             = 2048
  guest_agent     = true

  cloudinit_user_data = "#cloud-config\nhostname: web1\n"

  nics = [{
    name             = "lan"
    vnet             = data.vergeio_networks.lan.networks[0].id
    interface        = "virtio"
    assign_ipaddress = true
  }]

  tags = [{
    category    = "role"
    name        = "web"
    description = "Web servers"
  }]

  snapshot_profile = {
    name = "nightly"
    periods = [{
      name      = "nightly"
      frequency = "daily"
      hour      = 2
      minute    = 0
      retention = 604800
      quiesce   = true
    }]
  }
}
```

Recipe deploy:

```hcl
module "windows" {
  source = "../../modules/vm"

  name        = "windows-01"
  source_type = "recipe"
  recipe_id   = data.vergeio_vm_recipes.windows.recipes[0].id

  recipe_answers = {
    HOSTNAME    = "windows-01"
    YB_NIC_ETH0 = "Internal"
  }

  tags = [{
    category = "role"
    name     = "windows"
  }]
}
```

A runnable root module is in `examples/modules/vm`.

`terraform test` in this directory plans with a mocked provider. Lab apply tests are in `tests/lab`:

```bash
terraform test -test-directory=tests/lab
```
