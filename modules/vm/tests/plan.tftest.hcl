mock_provider "vergeio" {}

variables {
  name            = "web1"
  description     = "Web server"
  source_type     = "image"
  media_source_id = 25
  boot_disk_name  = "os"
  boot_disk_size  = 40
  os_family       = "linux"
  cpu_cores       = 2
  ram             = 2048
  machine_type    = "q35"
  enabled         = true
  powerstate      = false
  guest_agent     = true

  cloudinit_user_data = "#cloud-config\nhostname: web1\n"

  nics = [{
    name             = "lan"
    description      = "Application network"
    vnet             = 6
    interface        = "virtio"
    assign_ipaddress = true
  }]

  tag_ids = [9]

  snapshot_profile = {
    name        = "nightly"
    description = "Nightly VM snapshots"
    periods = [{
      name      = "nightly"
      frequency = "daily"
      hour      = 2
      minute    = 0
      retention = 604800
      quiesce   = true
      max_tier  = "1"
    }]
  }
}

run "image" {
  command = plan

  assert {
    condition     = length(vergeio_vm.image) == 1 && vergeio_vm.image[0].name == "web1" && vergeio_vm.image[0].os_family == "linux" && vergeio_vm.image[0].cpu_cores == 2 && vergeio_vm.image[0].ram == 2048 && vergeio_vm.image[0].machine_type == "q35" && vergeio_vm.image[0].guest_agent == true
    error_message = "The image VM was not planned from the module inputs."
  }

  assert {
    condition     = vergeio_vm.image[0].boot_disk.name == "os" && vergeio_vm.image[0].boot_disk.media == "import" && vergeio_vm.image[0].boot_disk.source == 25 && vergeio_vm.image[0].boot_disk.size == 40
    error_message = "The boot disk was not planned as an import of the media image."
  }

  assert {
    condition     = vergeio_vm.image[0].cloudinit_datasource == "nocloud" && vergeio_vm.image[0].cloudinit_files[0].name == "user-data" && vergeio_vm.image[0].cloudinit_files[0].contents == "#cloud-config\nhostname: web1\n" && vergeio_vm.image[0].cloudinit_files[1].name == "meta-data"
    error_message = "Cloud-init was not planned."
  }

  assert {
    condition     = vergeio_vm_nic.this["lan"].vnet == 6 && vergeio_vm_nic.this["lan"].interface == "virtio" && vergeio_vm_nic.this["lan"].assign_ipaddress == true
    error_message = "The NIC was not planned."
  }

  assert {
    condition     = vergeio_tag_member.this["9"].tag_id == 9 && length(vergeio_tag_member.this) == 1
    error_message = "Ansible inventory tags were not planned."
  }

  assert {
    condition     = vergeio_snapshot_profile.this[0].name == "nightly" && vergeio_snapshot_profile.this[0].period[0].frequency == "daily" && vergeio_snapshot_profile.this[0].period[0].retention == 604800 && length(vergeio_vm_recipe_instance.this) == 0
    error_message = "The snapshot profile was not planned, or a recipe instance was planned for an image VM."
  }
}

run "create_without_cloudinit" {
  command = apply

  variables {
    cloudinit_user_data = null
    snapshot_profile    = null
    tag_ids             = []
    nics                = []
  }
}

run "enable_cloudinit" {
  command = plan

  variables {
    cloudinit_user_data = "#cloud-config\nhostname: web1\n"
    snapshot_profile    = null
    tag_ids             = []
    nics                = []
  }

  assert {
    condition     = output.id == run.create_without_cloudinit.id && length(vergeio_vm.image) == 1 && vergeio_vm.image[0].cloudinit_files[0].contents == "#cloud-config\nhostname: web1\n"
    error_message = "Adding cloud-init replaced the VM."
  }
}

run "recipe" {
  command = plan

  variables {
    source_type         = "recipe"
    recipe_id           = "0123456789abcdef0123456789abcdef01234567"
    media_source_id     = null
    cloudinit_user_data = null
    nics                = []
    snapshot_profile    = null
    snapshot_profile_id = null
    recipe_answers = {
      HOSTNAME    = "web1"
      YB_NIC_ETH0 = "Internal"
    }
  }

  assert {
    condition     = vergeio_vm_recipe_instance.this[0].name == "web1" && vergeio_vm_recipe_instance.this[0].recipe_id == "0123456789abcdef0123456789abcdef01234567" && vergeio_vm_recipe_instance.this[0].answers["HOSTNAME"] == "web1" && length(vergeio_vm.image) == 0 && length(vergeio_snapshot_profile.this) == 0
    error_message = "The recipe VM was not planned, or an image VM was planned alongside it."
  }
}
