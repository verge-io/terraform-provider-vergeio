package network_test

import (
	"context"
	"fmt"
	"strconv"
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

// TestAccNetworkResource_MTU creates an internal network with a non-default
// MTU, updates that MTU, and requires a clean plan after each apply.
// State is compared with the network VergeOS returns, including layer2_id
// and layer2_type when those differ from the old hardcoded defaults.
func TestAccNetworkResource_MTU(t *testing.T) {
	networkName := acctest.Name("network-mtu")
	created := testAccNetworkMTUConfig(networkName, 9000)
	updated := testAccNetworkMTUConfig(networkName, 2000)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "mtu", "9000"),
					testAccCheckNetworkReadMatchesAPI("vergeio_network.test"),
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
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "mtu", "2000"),
					testAccCheckNetworkReadMatchesAPI("vergeio_network.test"),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
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

func testAccCheckNetworkReadMatchesAPI(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("invalid network id %q: %w", rs.Primary.ID, err)
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		network, err := client.Networks.Get(context.Background(), id)
		if err != nil {
			return err
		}

		if err := accAttrEqual(rs, "mtu", strconv.Itoa(network.MTU)); err != nil {
			return err
		}
		if err := accAttrEqual(rs, "layer2_id", strconv.Itoa(network.VLAN)); err != nil {
			return err
		}
		if err := accAttrEqual(rs, "layer2_type", network.Layer2Type); err != nil {
			return err
		}
		if err := accAttrEqual(rs, "enable_bonding", strconv.FormatBool(network.EnableBonding)); err != nil {
			return err
		}
		if network.InterfaceVnet.Int() == 0 {
			if got, ok := rs.Primary.Attributes["interface_vnet"]; ok && got != "" {
				return fmt.Errorf("interface_vnet = %q, API has no interface", got)
			}
		} else if err := accAttrEqual(rs, "interface_vnet", strconv.Itoa(network.InterfaceVnet.Int())); err != nil {
			return err
		}

		if network.MTU != 1500 && rs.Primary.Attributes["mtu"] == "1500" {
			return fmt.Errorf("mtu is the hardcoded 1500, API reports %d", network.MTU)
		}
		if network.VLAN != 0 && rs.Primary.Attributes["layer2_id"] == "0" {
			return fmt.Errorf("layer2_id is the hardcoded 0, API reports %d", network.VLAN)
		}
		if network.Layer2Type != "" && network.Layer2Type != "vlan" && rs.Primary.Attributes["layer2_type"] == "vlan" {
			return fmt.Errorf("layer2_type is the hardcoded vlan, API reports %q", network.Layer2Type)
		}
		return nil
	}
}

func accAttrEqual(rs *terraform.ResourceState, attr, want string) error {
	got, ok := rs.Primary.Attributes[attr]
	if !ok {
		return fmt.Errorf("state has no %s, API value is %q", attr, want)
	}
	if got != want {
		return fmt.Errorf("%s = %q, API value is %q", attr, got, want)
	}
	return nil
}

func testAccNetworkMTUConfig(networkName string, mtu int) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  powerstate = false
  mtu        = %d
}
`, networkName, mtu))
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
