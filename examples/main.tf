terraform {
	required_providers {
		vergeio = {
			source  = "vergeio/cloud/vergeio"
		}
	}
}
data "vergeio_version" "all" {}
data "vergeio_networks" "filtered" { filter_name = "Internal VM" }
data "vergeio_nodes" "filtered" { filter_name = "node1" }

resource "vergeio_vm" "terraform_test_vm" {
	name  = "terraform test vm"
	description = data.vergeio_version.all.version
	enabled = true
	os_family = "linux"
	cpu_cores = 8
	machine_type = "pc-q35-3.1"
	ram = "2048"
	preferred_node = 2
}

resource "vergeio_drive" "drive1" {
	machine = vergeio_vm.terraform_test_vm.machine
	name = "my boot disk"
	description = "std disk"
	disksize = 10000000000
}

resource "vergeio_drive" "cdrom1" {
	machine = vergeio_vm.terraform_test_vm.machine
	name = "cdrom1"
	media = "cdrom"
	media_source = 33
}

resource "vergeio_drive" "drive2" {
	machine = vergeio_vm.terraform_test_vm.machine
	name = "clone of existing disk"
	description = "clone"
	media = "clone"
	media_source = 38
}

resource "vergeio_nic" "nic1" {
	machine = vergeio_vm.terraform_test_vm.machine
	name = "nic1"
	description = ""
	vnet = data.vergeio_networks.filtered.networks[0].id
}