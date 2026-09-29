package vm_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccVMResource(t *testing.T) {
	vmName := acctest.Name("vm")
	basic := testAccVMResourceConfig(vmName, `
  enabled   = true
  cpu_cores = 2
  ram       = 2048
`)
	updated := testAccVMResourceConfig(vmName, `
  enabled     = true
  cpu_cores   = 4
  ram         = 4096
  description = "Updated Test VM"
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cpu_cores", "2"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "ram", "2048"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "id"),
				),
			},
			{
				Config: basic,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cpu_cores", "4"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "ram", "4096"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "description", "Updated Test VM"),
				),
			},
			{
				ResourceName:      "vergeio_vm.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckVMExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}
		return nil
	}
}

func testAccCheckVMDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_vm", func(ctx context.Context, id int) error {
		_, err := client.VMs.Get(ctx, id)
		return err
	})
}

func testAccVMResourceConfig(vmName, body string) string {
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm" "test" {
  name = %q
  %s
}
`, vmName, body))
}

// TestAccVMResource_DriveAndNIC imports a VM that already has a drive and a NIC.
// ImportStateVerify fails if Read does not put those blocks back into state.
func TestAccVMResource_DriveAndNIC(t *testing.T) {
	vmName := acctest.Name("vm")
	networkName := acctest.Name("network")
	config := testAccVMWithDriveAndNICConfig(vmName, networkName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMAndNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_drive.#", "1"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_drive.0.name", "os"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_drive.0.interface", "virtio-scsi"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_drive.0.disksize", "5"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_drive.0.orderid", "0"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_nic.#", "1"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_nic.0.name", "nic0"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_nic.0.interface", "virtio"),
					resource.TestCheckResourceAttrPair("vergeio_vm.test", "vergeio_nic.0.vnet", "vergeio_network.test", "id"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      "vergeio_vm.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckVMAndNetworkDestroy(s *terraform.State) error {
	if err := testAccCheckVMDestroy(s); err != nil {
		return err
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := client.Networks.Get(ctx, id)
		return err
	})
}

// TestAccVMResource_OmittedPowerStatePreservesExternalPowerOn creates a VM
// that never sets powerstate, powers it on outside Terraform, then changes
// an unrelated attribute. The VM must still be running afterward.
func TestAccVMResource_OmittedPowerStatePreservesExternalPowerOn(t *testing.T) {
	vmName := acctest.Name("vm")
	created := testAccVMResourceConfig(vmName, `
  enabled     = true
  cpu_cores   = 2
  ram         = 2048
  description = "v1"
`)
	updated := testAccVMResourceConfig(vmName, `
  enabled     = true
  cpu_cores   = 2
  ram         = 2048
  description = "v2"
`)

	var vmID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "description", "v1"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "false"),
					testAccCaptureVMID("vergeio_vm.test", &vmID),
				),
			},
			{
				PreConfig: func() {
					if err := testAccPowerOnVMOutsideTerraform(vmID); err != nil {
						t.Fatalf("power on VM outside Terraform: %v", err)
					}
				},
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("powerstate"), knownvalue.Bool(true)),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("description"), knownvalue.StringExact("v2")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "description", "v2"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "true"),
					testAccCheckVMPower("vergeio_vm.test", true),
				),
			},
		},
	})
}

func testAccCaptureVMID(resourceName string, id *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}
		*id = rs.Primary.ID
		return nil
	}
}

func testAccPowerOnVMOutsideTerraform(id string) error {
	vmID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("vm id %q: %w", id, err)
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := client.VMs.PowerOn(ctx, vmID); err != nil {
		return err
	}
	vm, err := client.VMs.Get(ctx, vmID)
	if err != nil {
		return err
	}
	if !vm.PowerState {
		return fmt.Errorf("vm %s is not running after power on", id)
	}
	return nil
}

func testAccCheckVMPower(resourceName string, running bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		vmID, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("vm id %q: %w", rs.Primary.ID, err)
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		vm, err := client.VMs.Get(context.Background(), vmID)
		if err != nil {
			return err
		}
		if vm.PowerState != running {
			return fmt.Errorf("vm %s powerstate = %v, want %v", rs.Primary.ID, vm.PowerState, running)
		}
		return nil
	}
}

// TestAccVMResource_ConsolePass creates a VM with a console password.
// The API does not return console_pass. The second plan must be empty, and
// changing the password must update the VM in place. A replacement deletes
// the VM's drives.
func TestAccVMResource_ConsolePass(t *testing.T) {
	vmName := acctest.Name("vm")
	original := testAccVMConsolePassConfig(vmName, "console-secret")
	rotated := testAccVMConsolePassConfig(vmName, "console-secret-rotated")
	var vmID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: original,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "console_pass_enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "console_pass", "console-secret"),
					testAccCaptureVMID("vergeio_vm.test", &vmID),
				),
			},
			{
				Config: original,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: rotated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "console_pass", "console-secret-rotated"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "console_pass_enabled", "true"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_vm.test"]
						if !ok {
							return fmt.Errorf("resource not found: vergeio_vm.test")
						}
						if rs.Primary.ID != vmID {
							return fmt.Errorf("vm id changed from %s to %s", vmID, rs.Primary.ID)
						}
						return nil
					},
				),
			},
			{
				Config: rotated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccVMConsolePassConfig(vmName, password string) string {
	return testAccVMResourceConfig(vmName, fmt.Sprintf(`
  enabled              = true
  cpu_cores            = 2
  ram                  = 2048
  console_pass_enabled = true
  console_pass         = %q
`, password))
}

func testAccVMWithDriveAndNICConfig(vmName, networkName string) string {
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
  powerstate = "false"
}

resource "vergeio_vm" "test" {
  name      = %q
  enabled   = true
  cpu_cores = 2
  ram       = 2048

  vergeio_drive {
    name      = "os"
    disksize  = 5
    interface = "virtio-scsi"
    orderid   = 0
  }

  vergeio_nic {
    name      = "nic0"
    interface = "virtio"
    vnet      = tonumber(vergeio_network.test.id)
  }
}
`, networkName, vmName))
}
