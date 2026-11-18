mock_provider "vergeio" {}

variables {
  name                = "lab1"
  shape               = "hci"
  vergeos_version     = "26.0.2"
  admin_password      = "Labpass1"
  admin_email         = "admin@example.com"
  external_network_id = 6
  external_ip         = "198.51.100.10"
  external_prefix     = 24
  external_gateway    = "198.51.100.1"
  external_dns        = "198.51.100.1"
  cpu_cores           = 2
  ram                 = 2048
  tier0_disk_count    = 1
  tier1_disk_count    = 1
  tier0_disk_size_gb  = 8
  tier1_disk_size_gb  = 8
  powerstate          = false
  network_powerstate  = false
}

run "hci" {
  command = plan

  assert {
    condition     = length(vergeio_vm.node) == 2 && vergeio_vm.node["controller-1"].name == "lab1-controller-1" && vergeio_vm.node["controller-2"].name == "lab1-controller-2" && vergeio_vm.node["controller-1"].os_description == "VergeOS 26.0.2" && vergeio_vm.node["controller-1"].nested_virtualization == true && vergeio_vm.node["controller-1"].disable_hypervisor == true && vergeio_vm.node["controller-1"].uefi == true && vergeio_vm.node["controller-1"].powerstate == false && vergeio_vm.node["controller-1"].machine_type == "q35" && vergeio_vm.node["controller-1"].shutdown_on_destroy == "kill" && vergeio_vm.node["controller-1"].cpu_cores == 2 && vergeio_vm.node["controller-1"].ram == 2048
    error_message = "The two controller VMs were not planned."
  }

  assert {
    condition     = vergeio_vm.node["controller-1"].cloudinit_datasource == "nocloud" && vergeio_vm.node["controller-1"].cloudinit_files[0].name == "user-data" && vergeio_vm.node["controller-1"].cloudinit_files[1].name == "meta-data" && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# vergeos_version=26.0.2") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "YC_VSAN_NEW=1") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "YC_CLOUD_NAME='lab1'") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "YC_USER_PASSWORD='Labpass1'") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "100.96.0.2/24") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "172.16.1.1/24") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "172.16.2.1/24") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "YC_DRIVE_LIST='/dev/sda /dev/sdb'") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "YC_VSAN_TIER_LIST='0 1'") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# create_storage=false") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# create_compute=false") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "198.51.100.10/24")
    error_message = "Controller 1's install seed was not planned."
  }

  assert {
    condition     = strcontains(nonsensitive(vergeio_vm.node["controller-2"].cloudinit_files[0].contents), "YC_VSAN_NEW=0") && strcontains(nonsensitive(vergeio_vm.node["controller-2"].cloudinit_files[0].contents), "YC_CLUSTER=1") && strcontains(nonsensitive(vergeio_vm.node["controller-2"].cloudinit_files[0].contents), "100.96.0.3/24") && strcontains(nonsensitive(vergeio_vm.node["controller-2"].cloudinit_files[0].contents), "172.16.1.2/24")
    error_message = "Controller 2's join seed was not planned."
  }

  assert {
    condition     = vergeio_network.core_1.name == "lab1-core-1" && vergeio_network.core_1.network == "172.16.1.0/24" && vergeio_network.core_1.ipaddress == "172.16.1.254" && vergeio_network.core_1.ipaddress_type == "none" && vergeio_network.core_1.mtu == 9142 && vergeio_network.core_1.dhcp_enabled == false && vergeio_network.core_1.powerstate == false && vergeio_network.core_2.name == "lab1-core-2" && vergeio_network.core_2.ipaddress == "172.16.2.254" && vergeio_network.core_2.network == "172.16.2.0/24"
    error_message = "The core fabric networks were not planned with a bare router address."
  }

  assert {
    condition     = length(vergeio_vm_drive.tier0) == 2 && vergeio_vm_drive.tier0["controller-1-tier0-0"].disksize == 8 && vergeio_vm_drive.tier0["controller-1-tier0-0"].interface == "virtio-scsi" && length(vergeio_vm_drive.tier1) == 2 && length(vergeio_vm_drive.compute) == 0 && length(vergeio_vm_drive.cdrom) == 0 && vergeio_vm_nic.external["controller-1"].vnet == 6 && vergeio_vm_nic.external["controller-1"].assign_ipaddress == false && length(vergeio_vm_nic.core_1) == 2 && length(vergeio_vm_nic.core_2) == 2
    error_message = "The disks and NICs were not planned."
  }

  assert {
    condition     = output.shape == "hci" && output.vergeos_version == "26.0.2" && output.ui_address == "198.51.100.10" && output.vm_names["controller-1"] == "lab1-controller-1"
    error_message = "The shape, version, and UI address were not planned."
  }
}

run "scale_out" {
  command = plan

  variables {
    shape           = "scale_out"
    scale_out_count = 1
  }

  assert {
    condition     = length(vergeio_vm.node) == 3 && vergeio_vm.node["scale-1"].name == "lab1-scale-1" && strcontains(nonsensitive(vergeio_vm.node["scale-1"].cloudinit_files[0].contents), "YC_INSTALL_TYPE=scale-out") && strcontains(nonsensitive(vergeio_vm.node["scale-1"].cloudinit_files[0].contents), "YC_NET_AUTODETECT=1") && strcontains(nonsensitive(vergeio_vm.node["scale-1"].cloudinit_files[0].contents), "YC_NET_EXTERNAL1_ADDR=dhcp") && length(vergeio_vm_drive.compute) == 0
    error_message = "The scale-out node was not planned."
  }
}

run "hybrid" {
  command = plan

  variables {
    shape                = "hybrid"
    compute_count        = 1
    compute_cpu_cores    = 2
    compute_ram          = 2048
    compute_disk_size_gb = 8
  }

  assert {
    condition     = length(vergeio_vm.node) == 3 && vergeio_vm.node["compute-1"].name == "lab1-compute-1" && vergeio_vm.node["compute-1"].cpu_cores == 2 && vergeio_vm.node["compute-1"].uefi == true && vergeio_vm_drive.compute["compute-1-disk"].disksize == 8 && vergeio_vm_drive.compute["compute-1-disk"].interface == "virtio-scsi" && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_INSTALL_TYPE=compute") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_CLUSTER=2") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_NO_TIME=1") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_SKIP_IPMI_CONFIG=1") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_SPLIT_LOCK_DETECTION=0") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_DRIVE_LIST='/dev/sda'") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# create_compute=true") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# create_storage=false") && length([for key, node in vergeio_vm.node : key if startswith(key, "storage-")]) == 0
    error_message = "The hybrid compute node was not planned."
  }
}

run "uci" {
  command = plan

  variables {
    shape                      = "uci"
    storage_count              = 1
    compute_count              = 1
    storage_tier1_disk_count   = 1
    storage_tier1_disk_size_gb = 8
    storage_cpu_cores          = 2
    storage_ram                = 2048
    compute_disk_size_gb       = 8
  }

  assert {
    condition     = length(vergeio_vm.node) == 4 && vergeio_vm.node["storage-1"].name == "lab1-storage-1" && vergeio_vm.node["compute-1"].name == "lab1-compute-1" && length([for key, drive in vergeio_vm_drive.tier1 : key if startswith(key, "controller-")]) == 0 && vergeio_vm_drive.tier1["storage-1-tier1-0"].disksize == 8 && strcontains(nonsensitive(vergeio_vm.node["storage-1"].cloudinit_files[0].contents), "YC_CLUSTER=2") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "YC_CLUSTER=3") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "YC_VSAN_TIER_LIST='0'") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# create_storage=true") && strcontains(nonsensitive(vergeio_vm.node["controller-1"].cloudinit_files[0].contents), "# create_compute=true") && strcontains(nonsensitive(vergeio_vm.node["compute-1"].cloudinit_files[0].contents), "100.96.0.5/24")
    error_message = "The uci storage and compute nodes were not planned."
  }
}

run "iso" {
  command = plan

  variables {
    media_source_id = 25
  }

  assert {
    condition     = length(vergeio_vm_drive.cdrom) == 2 && vergeio_vm_drive.cdrom["controller-1"].media == "cdrom" && vergeio_vm_drive.cdrom["controller-1"].media_source == 25 && vergeio_vm_drive.cdrom["controller-1"].interface == "ahci" && vergeio_vm.node["controller-1"].boot_order == "dc"
    error_message = "The install ISO was not planned as a CD-ROM."
  }
}
