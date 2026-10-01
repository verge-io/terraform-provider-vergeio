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

// TestAccNetworkResource_PowerState creates a network powered on, stops it,
// and powers it on again. Destroy runs while the network is running, which
// is the path that used to fail with "Network must be stopped to delete".
func TestAccNetworkResource_PowerState(t *testing.T) {
	networkName := acctest.Name("network-power")
	on := testAccNetworkPowerConfig(networkName, true)
	off := testAccNetworkPowerConfig(networkName, false)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: on,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "powerstate", "true"),
				),
			},
			{
				Config: on,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: off,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network.test", "powerstate", "false"),
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
			{
				Config: on,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network.test", "powerstate", "true"),
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

func testAccNetworkPowerConfig(networkName string, on bool) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	power := "false"
	if on {
		power = "true"
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = %s
}
`, networkName, power))
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
