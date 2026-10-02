package compute_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

// TestAccVMResource_NameOnly creates a VM that leaves cpu_cores and ram unset.
// Those attributes are created with the VergeOS defaults of 1 core and 1024 MiB.
func TestAccVMResource_NameOnly(t *testing.T) {
	vmName := acctest.Name("vm")
	config := testAccVMResourceConfig(vmName, "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cpu_cores", "1"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "ram", "1024"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "id"),
				),
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

// TestAccVMResource_DriveAndNIC creates a VM with a boot disk and a NIC resource.
// The NIC imports by its own id. The VM import ignores boot_disk because import
// only has the VM id and does not guess which drive is the boot disk.
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
					resource.TestCheckResourceAttr("vergeio_vm.test", "boot_disk.name", "os"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "boot_disk.size", "5"),
					resource.TestCheckResourceAttr("vergeio_vm_nic.test", "name", "nic0"),
					resource.TestCheckResourceAttr("vergeio_vm_nic.test", "interface", "virtio"),
					resource.TestCheckResourceAttrPair("vergeio_vm_nic.test", "vnet", "vergeio_network.test", "id"),
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
				ResourceName:            "vergeio_vm.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"boot_disk"},
			},
			{
				ResourceName:      "vergeio_vm_nic.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"assign_ipaddress",
				},
			},
			{
				ResourceName:      "vergeio_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccVMResource_DestroyRunningEmptyVMWithNIC creates a running VM with
// no drives and one NIC, then destroys it. The guest never boots, so it does
// not release the NIC. Destroy must still remove the NIC, the VM, and the network.
func TestAccVMResource_DestroyRunningEmptyVMWithNIC(t *testing.T) {
	vmName := acctest.Name("vm")
	networkName := acctest.Name("network")
	config := testAccRunningEmptyVMWithNICConfig(vmName, networkName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMAndNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					testAccCheckVMPower("vergeio_vm.test", true),
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_vm_nic.test", "name", "lan"),
					resource.TestCheckResourceAttrPair("vergeio_vm_nic.test", "vnet", "vergeio_network.test", "id"),
				),
			},
		},
	})
}

func testAccRunningEmptyVMWithNICConfig(vmName, networkName string) string {
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
  name                = %q
  enabled             = true
  cpu_cores           = 1
  ram                 = 1024
  powerstate          = true
  shutdown_on_destroy = "kill"
}

resource "vergeio_vm_nic" "test" {
  vm_id     = vergeio_vm.test.id
  name      = "lan"
  interface = "virtio"
  vnet      = tonumber(vergeio_network.test.id)
}
`, networkName, vmName))
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

// TestAccVMResource_PowerState powers a VM on and off from configuration,
// and corrects a power change made outside Terraform.
// An empty VM ignores ACPI. force_power_off stops it when configuration
// powers it off, and the simulated UI shutdown uses Kill for the same
// reason. The test is skipped unless TF_ACC=1.
func TestAccVMResource_PowerState(t *testing.T) {
	vmName := acctest.Name("vm")
	off := testAccVMPowerConfig(vmName, false)
	on := testAccVMPowerConfig(vmName, true)
	var vmID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: off,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "false"),
					testAccCheckVMPower("vergeio_vm.test", false),
					testAccCaptureVMID("vergeio_vm.test", &vmID),
				),
			},
			{
				Config: on,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("powerstate"), knownvalue.Bool(true)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "true"),
					testAccCheckVMPower("vergeio_vm.test", true),
				),
			},
			{
				// UI shutdown while configuration still wants the VM on.
				PreConfig: func() {
					if err := testAccPowerOffVMOutsideTerraform(vmID); err != nil {
						t.Fatalf("power off VM outside Terraform: %v", err)
					}
				},
				Config: on,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("powerstate"), knownvalue.Bool(true)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "true"),
					testAccCheckVMPower("vergeio_vm.test", true),
				),
			},
			{
				Config: off,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("powerstate"), knownvalue.Bool(false)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "false"),
					testAccCheckVMPower("vergeio_vm.test", false),
				),
			},
			{
				// UI power on while configuration wants the VM stopped.
				PreConfig: func() {
					if err := testAccPowerOnVMOutsideTerraform(vmID); err != nil {
						t.Fatalf("power on VM outside Terraform: %v", err)
					}
				},
				Config: off,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("powerstate"), knownvalue.Bool(false)),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "false"),
					testAccCheckVMPower("vergeio_vm.test", false),
				),
			},
			{
				Config: off,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccVMPowerConfig(vmName string, power bool) string {
	powerValue := "false"
	if power {
		powerValue = "true"
	}
	return testAccVMResourceConfig(vmName, fmt.Sprintf(`
  enabled         = true
  cpu_cores       = 1
  ram             = 1024
  powerstate      = %s
  force_power_off = true
  timeouts {
    update = "45s"
  }
`, powerValue))
}

// testAccPowerOffVMOutsideTerraform stops a VM outside Terraform.
// Empty test VMs ignore ACPI, so VMs.PowerOff waits until it times out.
// VMs.Kill cuts power immediately, the same hard stop a guest that never
// shuts down needs.
func testAccPowerOffVMOutsideTerraform(id string) error {
	vmID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("vm id %q: %w", id, err)
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := client.VMs.Kill(ctx, vmID); err != nil {
		return err
	}
	vm, err := client.VMs.Get(ctx, vmID)
	if err != nil {
		return err
	}
	if vm.PowerState {
		return fmt.Errorf("vm %s is still running after power off", id)
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
  powerstate = false
}

resource "vergeio_vm" "test" {
  name      = %q
  enabled   = true
  cpu_cores = 2
  ram       = 2048

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
`, networkName, vmName))
}

// TestAccVMResource_RenameDriveAndNIC renames a drive and a NIC. Both stay
// the same API objects: the drive key and the NIC id and MAC do not change,
// and the following plan is empty.
func TestAccVMResource_RenameDriveAndNIC(t *testing.T) {
	vmName := acctest.Name("vm")
	networkName := acctest.Name("network")
	created := testAccVMWithNamedDriveAndNICConfig(vmName, networkName, "os", "nic0")
	renamed := testAccVMWithNamedDriveAndNICConfig(vmName, networkName, "os-renamed", "lan")

	var driveKey, nicID, nicMAC string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMAndNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm_drive.test", "name", "os"),
					resource.TestCheckResourceAttr("vergeio_vm_nic.test", "name", "nic0"),
					testAccCaptureResourceAttr("vergeio_vm_drive.test", "id", &driveKey),
					testAccCaptureResourceAttr("vergeio_vm_nic.test", "id", &nicID),
					testAccCaptureResourceAttr("vergeio_vm_nic.test", "macaddress", &nicMAC),
				),
			},
			{
				Config: created,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: renamed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm_drive.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("vergeio_vm_nic.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_drive.test", "name", "os-renamed"),
					resource.TestCheckResourceAttr("vergeio_vm_nic.test", "name", "lan"),
					testAccExpectCapturedAttr("vergeio_vm_drive.test", "id", &driveKey),
					testAccExpectCapturedAttr("vergeio_vm_nic.test", "id", &nicID),
					testAccExpectCapturedAttr("vergeio_vm_nic.test", "macaddress", &nicMAC),
				),
			},
			{
				Config: renamed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccCaptureResourceAttr(resourceName, attr string, dest *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		value, err := testAccResourceAttr(s, resourceName, attr)
		if err != nil {
			return err
		}
		if value == "" {
			return fmt.Errorf("%s %s is empty", resourceName, attr)
		}
		*dest = value
		return nil
	}
}

func testAccExpectCapturedAttr(resourceName, attr string, want *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if want == nil || *want == "" {
			return fmt.Errorf("captured %s is empty", attr)
		}
		got, err := testAccResourceAttr(s, resourceName, attr)
		if err != nil {
			return err
		}
		if got != *want {
			return fmt.Errorf("%s %s = %q, want %q", resourceName, attr, got, *want)
		}
		return nil
	}
}

func testAccResourceAttr(s *terraform.State, resourceName, attr string) (string, error) {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return "", fmt.Errorf("resource not found: %s", resourceName)
	}
	value, ok := rs.Primary.Attributes[attr]
	if !ok {
		return "", fmt.Errorf("%s has no attribute %s", resourceName, attr)
	}
	return value, nil
}

func testAccVMWithNamedDriveAndNICConfig(vmName, networkName, driveName, nicName string) string {
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
  name      = %q
  enabled   = true
  cpu_cores = 2
  ram       = 2048
}

resource "vergeio_vm_drive" "test" {
  vm_id     = vergeio_vm.test.id
  name      = %q
  disksize  = 5
  interface = "virtio-scsi"
  orderid   = 0
}

resource "vergeio_vm_nic" "test" {
  vm_id     = vergeio_vm.test.id
  name      = %q
  interface = "virtio"
  vnet      = tonumber(vergeio_network.test.id)
}
`, networkName, vmName, driveName, nicName))
}

// TestAccVMResource_AddDevice creates a VM, then adds a TPM device.
// The device key, machine, and TPM ids are unknown in that plan. Apply
// stores the values VergeOS assigns, and the following plan is empty.
func TestAccVMResource_AddDevice(t *testing.T) {
	vmName := acctest.Name("vm")
	without := testAccVMResourceConfig(vmName, `
  enabled    = true
  cpu_cores  = 1
  ram        = 1024
  powerstate = false
`)
	withDevice := testAccVMResourceConfig(vmName, `
  enabled    = true
  cpu_cores  = 1
  ram        = 1024
  powerstate = false

  vergeio_device {
    name = "tpm"
    type = "tpm"
    tpm_settings = {
      model   = "crb"
      version = "2.0"
    }
  }
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: without,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckNoResourceAttr("vergeio_vm.test", "vergeio_device.0.key"),
				),
			},
			{
				Config: without,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: withDevice,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue("vergeio_vm.test", tfjsonpath.New("vergeio_device").AtSliceIndex(0).AtMapKey("key")),
						plancheck.ExpectUnknownValue("vergeio_vm.test", tfjsonpath.New("vergeio_device").AtSliceIndex(0).AtMapKey("machine")),
						plancheck.ExpectUnknownValue("vergeio_vm.test", tfjsonpath.New("vergeio_device").AtSliceIndex(0).AtMapKey("tpm_settings").AtMapKey("key")),
						plancheck.ExpectUnknownValue("vergeio_vm.test", tfjsonpath.New("vergeio_device").AtSliceIndex(0).AtMapKey("tpm_settings").AtMapKey("machine_device")),
						// The plan keeps the configured label. Rewriting it to the stored
						// key "2" is an invalid plan.
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("vergeio_device").AtSliceIndex(0).AtMapKey("tpm_settings").AtMapKey("version"), knownvalue.StringExact("2.0")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.#", "1"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.name", "tpm"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.type", "tpm"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.tpm_settings.model", "crb"),
					// SemanticEquals keeps the configured label. Create sends stored "2".
					// The settings update omits version because it is read-only.
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.tpm_settings.version", "2.0"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "vergeio_device.0.key"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "vergeio_device.0.machine"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "vergeio_device.0.tpm_settings.key"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "vergeio_device.0.tpm_settings.machine_device"),
				),
			},
			{
				Config: withDevice,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccVMResource_TPMVersionReplace creates a VM with TPM version 2,
// then changes version to 1.2. VergeOS cannot update version in place, so
// the plan must replace the VM (RequiresReplace) instead of a doomed
// in-place update that leaves version unchanged.
func TestAccVMResource_TPMVersionReplace(t *testing.T) {
	vmName := acctest.Name("vm-tpm-ver")
	withV2 := testAccVMResourceConfig(vmName, `
  enabled    = true
  cpu_cores  = 1
  ram        = 1024
  powerstate = false

  vergeio_device {
    name = "tpm"
    type = "tpm"
    tpm_settings = {
      model   = "crb"
      version = "2"
    }
  }
`)
	withV12 := testAccVMResourceConfig(vmName, `
  enabled    = true
  cpu_cores  = 1
  ram        = 1024
  powerstate = false

  vergeio_device {
    name = "tpm"
    type = "tpm"
    tpm_settings = {
      model   = "crb"
      version = "1.2"
    }
  }
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: withV2,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.tpm_settings.version", "2"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.tpm_settings.model", "crb"),
				),
			},
			{
				Config: withV2,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: withV12,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionReplace),
						plancheck.ExpectKnownValue("vergeio_vm.test", tfjsonpath.New("vergeio_device").AtSliceIndex(0).AtMapKey("tpm_settings").AtMapKey("version"), knownvalue.StringExact("1.2")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.tpm_settings.version", "1.2"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "vergeio_device.0.tpm_settings.model", "crb"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "vergeio_device.0.key"),
				),
			},
			{
				Config: withV12,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccVMResource_CloudInitFilesExternalDelete covers #188: wiping every
// configured cloud-init file outside Terraform must plan a recreate when the
// VM was created with powerstate=false (files still expected live). Deleting
// one of several files is still detected.
func TestAccVMResource_CloudInitFilesExternalDelete(t *testing.T) {
	vmName := acctest.Name("vm-ci188")
	oneFile := testAccVMCloudInitConfig(vmName, false, false)
	twoFiles := testAccVMCloudInitConfig(vmName, true, false)

	var vmID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: oneFile,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cloudinit_files.#", "1"),
					testAccCaptureVMID("vergeio_vm.test", &vmID),
				),
			},
			{
				PreConfig: func() {
					if err := testAccDeleteAllCloudInitFilesOutsideTerraform(vmID); err != nil {
						t.Fatalf("delete all cloud-init files: %v", err)
					}
				},
				Config: oneFile,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "cloudinit_files.#", "1"),
					testAccCheckVMCloudInitFileCount("vergeio_vm.test", 1),
				),
			},
			{
				Config: twoFiles,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "cloudinit_files.#", "2"),
					testAccCheckVMCloudInitFileCount("vergeio_vm.test", 2),
					testAccCaptureVMID("vergeio_vm.test", &vmID),
				),
			},
			{
				PreConfig: func() {
					if err := testAccDeleteCloudInitFileOutsideTerraform(vmID, "/meta-data"); err != nil {
						t.Fatalf("delete one cloud-init file: %v", err)
					}
				},
				Config: twoFiles,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "cloudinit_files.#", "2"),
					testAccCheckVMCloudInitFileCount("vergeio_vm.test", 2),
				),
			},
			{
				PreConfig: func() {
					if err := testAccDeleteAllCloudInitFilesOutsideTerraform(vmID); err != nil {
						t.Fatalf("delete all cloud-init files: %v", err)
					}
				},
				Config: twoFiles,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm.test", "cloudinit_files.#", "2"),
					testAccCheckVMCloudInitFileCount("vergeio_vm.test", 2),
				),
			},
		},
	})
}

// TestAccVMResource_CloudInitFilesPowerOnDetachEmptyPlan covers the #185 path
// preserved by #188: after create with powerstate=true the provider detaches
// cloud-init files, state still lists them, and the next plan stays empty.
func TestAccVMResource_CloudInitFilesPowerOnDetachEmptyPlan(t *testing.T) {
	vmName := acctest.Name("vm-ci188-pon")
	poweredOn := testAccVMCloudInitConfig(vmName, false, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: poweredOn,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cloudinit_files.#", "1"),
					testAccCheckVMCloudInitFileCount("vergeio_vm.test", 0),
				),
			},
			{
				Config: poweredOn,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccVMCloudInitConfig(vmName string, two, powerOn bool) string {
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	power := "false"
	if powerOn {
		power = "true"
	}
	files := `
  cloudinit_files = [
    { name = "/user-data", contents = "#cloud-config\nhostname: third\n" },
  ]`
	if two {
		files = `
  cloudinit_files = [
    { name = "/user-data", contents = "#cloud-config\nhostname: third\n" },
    { name = "/meta-data", contents = "instance-id: x\n" },
  ]`
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm" "test" {
  name                 = %q
  cpu_cores            = 1
  ram                  = 1024
  powerstate           = %s
  cloudinit_datasource = "nocloud"
  %s
}
`, vmName, power, files))
}

func testAccDeleteAllCloudInitFilesOutsideTerraform(id string) error {
	vmID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("vm id %q: %w", id, err)
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	files, err := client.CloudInitFiles.ListByVM(ctx, vmID)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := client.CloudInitFiles.Delete(ctx, file.Key.Int()); err != nil {
			return fmt.Errorf("delete cloud-init file %q: %w", file.Name, err)
		}
	}
	return nil
}

func testAccDeleteCloudInitFileOutsideTerraform(id, name string) error {
	vmID, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("vm id %q: %w", id, err)
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	files, err := client.CloudInitFiles.ListByVM(ctx, vmID)
	if err != nil {
		return err
	}
	want := strings.TrimPrefix(name, "/")
	deleted := false
	for _, file := range files {
		if strings.TrimPrefix(file.Name, "/") == want {
			if err := client.CloudInitFiles.Delete(ctx, file.Key.Int()); err != nil {
				return fmt.Errorf("delete cloud-init file %q: %w", file.Name, err)
			}
			deleted = true
		}
	}
	if !deleted {
		return fmt.Errorf("cloud-init file %q not found on vm %s", name, id)
	}
	return nil
}

func testAccCheckVMCloudInitFileCount(resourceName string, want int) resource.TestCheckFunc {
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
		files, err := client.CloudInitFiles.ListByVM(context.Background(), vmID)
		if err != nil {
			return err
		}
		if len(files) != want {
			return fmt.Errorf("vm %s has %d cloud-init files, want %d", rs.Primary.ID, len(files), want)
		}
		return nil
	}
}

// TestAccVMResource_BootDiskMediaSourceAdopt covers the import (and 2.x
// upgrade) path for a boot disk that declares media and source. Import leaves
// boot_disk null; the next apply must adopt by name with 0 destroy instead of
// RequiresReplace on null→media/source.
//
// Create uses media alone (VergeOS rejects media_source on media=disk). After
// import, configuration adds both media and source so the plan hits both
// RequiresReplaceIf modifiers the way a 2.x import/upgrade config does.
func TestAccVMResource_BootDiskMediaSourceAdopt(t *testing.T) {
	vmName := acctest.Name("vm-bd-media")
	created := testAccVMBootDiskMediaOnlyConfig(vmName)
	adopt := testAccVMBootDiskMediaSourceConfig(vmName)

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
					resource.TestCheckResourceAttr("vergeio_vm.test", "boot_disk.name", "os"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "boot_disk.media", "disk"),
					testAccCaptureResourceAttr("vergeio_vm.test", "id", &vmID),
				),
			},
			{
				ResourceName:            "vergeio_vm.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"boot_disk"},
			},
			{
				Config: adopt,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_vm.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccExpectCapturedAttr("vergeio_vm.test", "id", &vmID),
					resource.TestCheckResourceAttr("vergeio_vm.test", "boot_disk.name", "os"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "boot_disk.media", "disk"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "boot_disk.source"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "boot_disk.key"),
				),
			},
			{
				Config: adopt,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccVMBootDiskMediaOnlyConfig(vmName string) string {
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm" "test" {
  name      = %q
  enabled   = true
  cpu_cores = 1
  ram       = 1024

  boot_disk {
    name  = "os"
    size  = 5
    media = "disk"
  }
}
`, vmName))
}

func testAccVMBootDiskMediaSourceConfig(vmName string) string {
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
data "vergeio_mediasources" "img" {
  filter_name = "debian13-golden.ova"
}

resource "vergeio_vm" "test" {
  name      = %q
  enabled   = true
  cpu_cores = 1
  ram       = 1024

  boot_disk {
    name   = "os"
    size   = 5
    media  = "disk"
    source = data.vergeio_mediasources.img.mediasources[0].id
  }
}
`, vmName))
}
