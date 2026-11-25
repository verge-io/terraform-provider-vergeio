package network_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccNetworkRule_OrderID(t *testing.T) {
	networkName := acctest.Name("rule-order")
	ruleA := acctest.Name("rule-a")
	ruleB := acctest.Name("rule-b")
	ruleC := acctest.Name("rule-c")
	created := testAccNetworkRuleOrderConfig(networkName, ruleA, ruleB, ruleC, 1, 2, 3)
	swapped := testAccNetworkRuleOrderConfig(networkName, ruleA, ruleB, ruleC, 3, 2, 1)
	spaced := testAccNetworkRuleOrderConfig(networkName, ruleA, ruleB, ruleC, 10, 20, 30)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkRuleOrderDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network_rule.a", "name", ruleA),
					resource.TestCheckResourceAttr("vergeio_network_rule.b", "name", ruleB),
					resource.TestCheckResourceAttr("vergeio_network_rule.c", "name", ruleC),
					resource.TestCheckResourceAttr("vergeio_network_rule.a", "orderid", "1"),
					resource.TestCheckResourceAttr("vergeio_network_rule.b", "orderid", "2"),
					resource.TestCheckResourceAttr("vergeio_network_rule.c", "orderid", "3"),
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
				Config: swapped,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rule.a", "orderid", "3"),
					resource.TestCheckResourceAttr("vergeio_network_rule.b", "orderid", "2"),
					resource.TestCheckResourceAttr("vergeio_network_rule.c", "orderid", "1"),
				),
			},
			{
				Config: swapped,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config:      spaced,
				ExpectError: regexp.MustCompile(`outside 1 through \d+`),
			},
		},
	})
}

func testAccNetworkRuleOrderConfig(networkName, ruleA, ruleB, ruleC string, orderA, orderB, orderC int) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(ruleA); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(ruleB); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(ruleC); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = false
}

resource "vergeio_network_rule" "a" {
  vnet              = vergeio_network.test.id
  name              = %q
  protocol          = "tcp"
  destination_ports = "22"
  orderid           = %d
  apply             = false
}

resource "vergeio_network_rule" "b" {
  vnet              = vergeio_network.test.id
  name              = %q
  protocol          = "tcp"
  destination_ports = "80"
  orderid           = %d
  apply             = false

  depends_on = [vergeio_network_rule.a]
}

resource "vergeio_network_rule" "c" {
  vnet              = vergeio_network.test.id
  name              = %q
  protocol          = "udp"
  destination_ports = "53"
  orderid           = %d
  apply             = false

  depends_on = [vergeio_network_rule.b]
}
`, networkName, ruleA, orderA, ruleB, orderB, ruleC, orderC))
}

func testAccCheckNetworkRuleOrderDestroy(s *terraform.State) error {
	if err := testAccCheckNetworkDestroy(s); err != nil {
		return err
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	for _, name := range []string{"vergeio_network_rule.a", "vergeio_network_rule.b", "vergeio_network_rule.c"} {
		rs, ok := s.RootModule().Resources[name]
		if !ok || rs.Primary.ID == "" {
			continue
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("invalid rule id %q: %w", rs.Primary.ID, err)
		}
		_, err = client.VNetRules.Get(context.Background(), id)
		if err == nil {
			return fmt.Errorf("firewall rule %s (%d) still exists", name, id)
		}
		if !vergeos.IsNotFoundError(err) {
			return err
		}
	}
	return nil
}
