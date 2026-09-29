package network_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccNetworkResource(t *testing.T) {
	networkName := acctest.Name("network")
	basic := testAccNetworkResourceConfig(networkName, "enabled = true")
	updated := testAccNetworkResourceConfig(networkName, "enabled = false")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "type", "internal"),
					resource.TestCheckResourceAttr("vergeio_network.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_network.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_network.test", "id"),
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
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "enabled", "false"),
				),
			},
			{
				ResourceName:      "vergeio_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckNetworkExists(resourceName string) resource.TestCheckFunc {
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

func testAccCheckNetworkDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := client.Networks.Get(ctx, id)
		return err
	})
}

func testAccNetworkResourceConfig(networkName, extra string) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  powerstate = false
  %s
}
`, networkName, extra))
}
