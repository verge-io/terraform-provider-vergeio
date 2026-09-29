package vm_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

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
