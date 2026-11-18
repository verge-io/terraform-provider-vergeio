# vm

Creates a VM from a media image or a catalog recipe, with cloud-init, Ansible inventory tags, and an optional snapshot profile.

`source_type` `image` creates one `vergeio_vm`. Set `media_source_id` to import a media image. Omit it and set `boot_disk_size` for a blank disk. Cloud-init is `user-data` and `meta-data` on that same VM. Both bodies are stored in state. Omit `cloudinit_user_data` for an empty cloud-config. Setting or clearing it updates the file in place. NICs are `vergeio_vm_nic`. `vnet` is the network key as a number.

`source_type` `recipe` creates `vergeio_vm_recipe_instance`. `recipe_id` is the 40-character key from `vergeio_vm_recipes`. Guest hostname, disk size, and network answers go in `recipe_answers`. That resource has no `cloudinit_files`, NIC, or `snapshot_profile` attribute, so those inputs stay on image VMs.

`tag_ids` assigns tags that already exist. The map key is a name you choose and must be known at plan time. The value is the tag key and may be unknown until apply. Create the category and the tag outside this module so two VMs can share them. Deleting a category deletes its tags and every assignment of those tags.

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
  tag_ids = {
    web = tonumber(vergeio_tag.web.id)
  }

  nics = [{
    name             = "lan"
    vnet             = data.vergeio_networks.lan.networks[0].id
    interface        = "virtio"
    assign_ipaddress = true
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
}
```

A runnable root module is in `examples/modules/vm`.

`terraform test` in this directory plans with a mocked provider. The lab apply test is a separate root in `tests/lab` and is not part of that run:

```bash
terraform -chdir=tests/lab init -backend=false
terraform -chdir=tests/lab test
```
