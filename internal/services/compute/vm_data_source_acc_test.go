package compute_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccVMDataSource_DriveAndNIC(t *testing.T) {
	vmName := acctest.Name("vm")
	networkName := acctest.Name("network")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMAndNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMDataSourceDriveAndNICConfig(vmName, networkName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.#", "1"),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.name", vmName),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.is_snapshot", "false"),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.drives.#", "1"),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.drives.0.name", "os"),
					resource.TestCheckResourceAttrSet("data.vergeio_vms.test", "vms.0.drives.0.key"),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.nics.#", "1"),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.nics.0.name", "nic0"),
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "vms.0.nics.0.interface", "virtio"),
					resource.TestCheckResourceAttrSet("data.vergeio_vms.test", "vms.0.nics.0.key"),
					resource.TestCheckResourceAttrSet("data.vergeio_vms.test", "vms.0.nics.0.macaddress"),
					resource.TestCheckResourceAttrPair("data.vergeio_vms.test", "vms.0.nics.0.vnet", "vergeio_vm_nic.test", "vnet"),
				),
			},
		},
	})
}

func testAccVMDataSourceDriveAndNICConfig(vmName, networkName string) string {
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = false
}

resource "vergeio_vm" "test" {
  name       = %q
  enabled    = true
  cpu_cores  = 1
  ram        = 1024
  uefi       = false
  powerstate = false

  boot_disk {
    name = "os"
    size = 5
  }
}

resource "vergeio_vm_nic" "test" {
  vm_id     = vergeio_vm.test.id
  name      = "nic0"
  interface = "virtio"
  vnet      = tonumber(vergeio_network.test.id)
}

data "vergeio_vms" "test" {
  filter_name = vergeio_vm.test.name
  is_snapshot = false

  depends_on = [vergeio_vm_nic.test]
}
`, networkName, vmName))
}
