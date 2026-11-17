package network_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccNetworkRules_ReorderInsertAndRestore changes the rule list after
// create. Reorder, insert, remove, and restoring a list from empty used to
// apply in VergeOS and then fail with an inconsistent id or orderid.
func TestAccNetworkRules_ReorderInsertAndRestore(t *testing.T) {
	networkName := acctest.Name("network")
	created := testAccNetworkRulesConfig(networkName, accRuleSSH, accRuleWeb, accRuleDNS)
	reordered := testAccNetworkRulesConfig(networkName, accRuleDNS, accRuleSSH, accRuleWeb)
	inserted := testAccNetworkRulesConfig(networkName, accRuleDNS, accRuleICMP, accRuleSSH, accRuleWeb)
	removed := testAccNetworkRulesConfig(networkName, accRuleDNS, accRuleSSH)
	empty := testAccNetworkRulesConfig(networkName)
	restored := testAccNetworkRulesConfig(networkName, accRuleSSH, accRuleWeb, accRuleDNS)

	var sshID, webID, dnsID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.#", "3"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.0.name", "ssh"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.1.name", "web"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.2.name", "dns"),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.0.id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.0.orderid"),
					testAccCaptureResourceAttr("vergeio_network_rules.test", "rule.0.id", &sshID),
					testAccCaptureResourceAttr("vergeio_network_rules.test", "rule.1.id", &webID),
					testAccCaptureResourceAttr("vergeio_network_rules.test", "rule.2.id", &dnsID),
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
				Config: reordered,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_rules.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(0).AtMapKey("orderid")),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(1).AtMapKey("orderid")),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(2).AtMapKey("orderid")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.0.name", "dns"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.1.name", "ssh"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.2.name", "web"),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.0.id", &dnsID),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.1.id", &sshID),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.2.id", &webID),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.0.orderid"),
				),
			},
			{
				Config: reordered,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: inserted,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(1).AtMapKey("id")),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(1).AtMapKey("orderid")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.#", "4"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.0.name", "dns"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.1.name", "icmp"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.2.name", "ssh"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.3.name", "web"),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.0.id", &dnsID),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.2.id", &sshID),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.3.id", &webID),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.1.id"),
				),
			},
			{
				Config: removed,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.#", "2"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.0.name", "dns"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.1.name", "ssh"),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.0.id", &dnsID),
					testAccExpectCapturedAttr("vergeio_network_rules.test", "rule.1.id", &sshID),
				),
			},
			{
				Config: empty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.#", "0"),
				),
			},
			{
				Config: empty,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: restored,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(0).AtMapKey("id")),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(0).AtMapKey("orderid")),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(1).AtMapKey("id")),
						plancheck.ExpectUnknownValue("vergeio_network_rules.test", tfjsonpath.New("rule").AtSliceIndex(2).AtMapKey("id")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.#", "3"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.0.name", "ssh"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.1.name", "web"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.2.name", "dns"),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.0.id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.0.orderid"),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.1.id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rules.test", "rule.2.id"),
				),
			},
			{
				Config: restored,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

const (
	accRuleSSH = `    {
      name              = "ssh"
      protocol          = "tcp"
      destination_ports = "22"
    }`
	accRuleWeb = `    {
      name              = "web"
      protocol          = "tcp"
      destination_ports = "80,443"
    }`
	accRuleDNS = `    {
      name              = "dns"
      protocol          = "udp"
      destination_ports = "53"
    }`
	accRuleICMP = `    {
      name     = "icmp"
      protocol = "icmp"
    }`
)

func testAccNetworkRulesConfig(networkName string, rules ...string) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	body := ""
	for i, rule := range rules {
		if i > 0 {
			body += ",\n"
		}
		body += rule
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = false
}

resource "vergeio_network_rules" "test" {
  vnet = vergeio_network.test.id

  rule = [
%s
  ]
}
`, networkName, body))
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
